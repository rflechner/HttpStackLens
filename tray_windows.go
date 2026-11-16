//go:build windows

package main

import (
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The notification icon has its own hidden window and message loop. Wails
// keeps ownership of the main UI thread; all tray handles stay on this thread.
const trayWindowClass = "HttpStackLens.NotificationArea"
const (
	trayNotifyMessage = 0x8001 // WM_APP + 1
	trayShowMessage   = 0x8002
	wmClose           = 0x0010
	wmDestroy         = 0x0002
)

var (
	trayUser32   = windows.NewLazySystemDLL("user32.dll")
	trayShell32  = windows.NewLazySystemDLL("shell32.dll")
	trayKernel32 = windows.NewLazySystemDLL("kernel32.dll")
	trayCurrent  *windowsTray // accessed only by the tray message-loop thread
	trayCallback = syscall.NewCallback(trayWindowProc)
)

// Win32 ABI structures: pointer-sized handles are uintptr on both amd64 and
// arm64. The full NOTIFYICONDATAW layout also works on 32-bit Windows.
type trayWindowClassEx struct {
	Size, Style                        uint32
	WndProc                            uintptr
	ClsExtra, WndExtra                 int32
	Instance, Icon, Cursor, Background uintptr
	MenuName, ClassName                *uint16
	IconSmall                          uintptr
}

type trayPoint struct{ X, Y int32 }
type trayMessage struct {
	Window         uintptr
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	Point          trayPoint
	Private        uint32
}

type trayNotifyIconData struct {
	Size                       uint32
	Window                     uintptr
	ID, Flags, CallbackMessage uint32
	Icon                       uintptr
	Tip                        [128]uint16
	State, StateMask           uint32
	Info                       [256]uint16
	Version                    uint32
	InfoTitle                  [64]uint16
	InfoFlags                  uint32
	GUID                       windows.GUID
	BalloonIcon                uintptr
}

type windowsTray struct {
	window         uintptr
	icon           uintptr
	ownedIcon      bool
	notification   trayNotifyIconData
	taskbarCreated uint32
	show, quit     func()
	startup        *desktopStartupController
}

func trayText(value string) *uint16 { return windows.StringToUTF16Ptr(value) }

func acquireDesktopInstance(background bool) (func(), bool, error) {
	mutex, err := windows.CreateMutex(nil, false, trayText(`Local\HttpStackLens.Desktop`))
	if err == windows.ERROR_ALREADY_EXISTS {
		_ = windows.CloseHandle(mutex)
		if !background {
			// At sign-in, the first instance may still be loading its WebView.
			// Give its notification window time to appear before sending the request.
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				window, _, _ := trayUser32.NewProc("FindWindowW").Call(uintptr(unsafe.Pointer(trayText(trayWindowClass))), 0)
				if window != 0 {
					var processID uint32
					trayUser32.NewProc("GetWindowThreadProcessId").Call(window, uintptr(unsafe.Pointer(&processID)))
					if processID != 0 {
						trayUser32.NewProc("AllowSetForegroundWindow").Call(uintptr(processID))
					}
					trayUser32.NewProc("PostMessageW").Call(window, trayShowMessage, 0, 0)
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
		}
		return func() {}, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return func() { _ = windows.CloseHandle(mutex) }, true, nil
}

func startDesktopTray(show, quit func(), startup *desktopStartupController) (func(), error) {
	tray := &windowsTray{show: show, quit: quit, startup: startup}
	ready := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(done)
		trayCurrent = tray
		defer func() { trayCurrent = nil }()
		if err := tray.create(); err != nil {
			ready <- err
			return
		}
		defer tray.destroy()
		ready <- nil
		var message trayMessage
		for {
			result, _, err := trayUser32.NewProc("GetMessageW").Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
			if int32(result) <= 0 {
				if int32(result) < 0 {
					slog.Error("Notification icon message loop failed", "error", err)
					go quit()
				}
				return
			}
			trayUser32.NewProc("TranslateMessage").Call(uintptr(unsafe.Pointer(&message)))
			trayUser32.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&message)))
		}
	}()
	if err := <-ready; err != nil {
		<-done
		return nil, err
	}
	window := tray.window
	var once sync.Once
	return func() {
		once.Do(func() {
			trayUser32.NewProc("PostMessageW").Call(window, wmClose, 0, 0)
			<-done
		})
	}, nil
}

func (t *windowsTray) create() error {
	instance, _, _ := trayKernel32.NewProc("GetModuleHandleW").Call(0)
	class := trayWindowClassEx{WndProc: trayCallback, Instance: instance, ClassName: trayText(trayWindowClass)}
	class.Size = uint32(unsafe.Sizeof(class))
	if result, _, err := trayUser32.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&class))); result == 0 {
		return fmt.Errorf("register notification window: %w", err)
	}
	window, _, err := trayUser32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(class.ClassName)), uintptr(unsafe.Pointer(trayText("HttpStackLens"))), 0, 0, 0, 0, 0, 0, 0, instance, 0)
	if window == 0 {
		trayUser32.NewProc("UnregisterClassW").Call(uintptr(unsafe.Pointer(class.ClassName)), instance)
		return fmt.Errorf("create notification window: %w", err)
	}
	t.window = window
	t.loadIcon()
	t.notification = trayNotifyIconData{Window: window, ID: 1, Flags: 7, CallbackMessage: trayNotifyMessage, Icon: t.icon}
	t.notification.Size = uint32(unsafe.Sizeof(t.notification))
	copy(t.notification.Tip[:], windows.StringToUTF16("HttpStackLens — local development proxy"))
	message, _, _ := trayUser32.NewProc("RegisterWindowMessageW").Call(uintptr(unsafe.Pointer(trayText("TaskbarCreated"))))
	t.taskbarCreated = uint32(message)
	if err := t.addIcon(); err != nil {
		t.destroy()
		return err
	}
	return nil
}

func (t *windowsTray) loadIcon() {
	// Packaged builds already contain the application icon. Development builds
	// without resources use Windows' shared application icon instead.
	if executable, err := os.Executable(); err == nil {
		count, _, _ := trayShell32.NewProc("ExtractIconExW").Call(uintptr(unsafe.Pointer(trayText(executable))), 0, 0, uintptr(unsafe.Pointer(&t.icon)), 1)
		t.ownedIcon = count > 0 && t.icon != 0
	}
	if !t.ownedIcon {
		t.icon, _, _ = trayUser32.NewProc("LoadIconW").Call(0, 32512) // IDI_APPLICATION
	}
}

func (t *windowsTray) addIcon() error {
	if result, _, err := trayShell32.NewProc("Shell_NotifyIconW").Call(0, uintptr(unsafe.Pointer(&t.notification))); result == 0 {
		return fmt.Errorf("add notification icon: %w", err)
	}
	return nil
}

func (t *windowsTray) destroy() {
	trayShell32.NewProc("Shell_NotifyIconW").Call(2, uintptr(unsafe.Pointer(&t.notification))) // NIM_DELETE
	trayUser32.NewProc("DestroyWindow").Call(t.window)
	if t.ownedIcon {
		trayUser32.NewProc("DestroyIcon").Call(t.icon)
	}
	instance, _, _ := trayKernel32.NewProc("GetModuleHandleW").Call(0)
	trayUser32.NewProc("UnregisterClassW").Call(uintptr(unsafe.Pointer(trayText(trayWindowClass))), instance)
}

func trayWindowProc(window uintptr, message uint32, wParam, lParam uintptr) uintptr {
	t := trayCurrent
	if t != nil {
		switch message {
		case trayShowMessage:
			go t.show()
			return 0
		case trayNotifyMessage:
			switch uint32(lParam) {
			case 0x0202, 0x0203: // WM_LBUTTONUP / WM_LBUTTONDBLCLK
				go t.show()
			case 0x0205, 0x007b: // WM_RBUTTONUP / WM_CONTEXTMENU
				t.menu()
			}
			return 0
		case wmClose:
			// Cleanup is performed once, after the message loop has stopped.
			trayUser32.NewProc("PostQuitMessage").Call(0)
			return 0
		case wmDestroy:
			return 0
		}
		if t.taskbarCreated != 0 && message == t.taskbarCreated {
			if err := t.addIcon(); err != nil {
				slog.Error("Could not restore notification icon after Explorer restart", "error", err)
				go t.show()
			}
			return 0
		}
	}
	result, _, _ := trayUser32.NewProc("DefWindowProcW").Call(window, uintptr(message), wParam, lParam)
	return result
}

func (t *windowsTray) menu() {
	menu, _, err := trayUser32.NewProc("CreatePopupMenu").Call()
	if menu == 0 {
		slog.Error("Could not open notification menu", "error", err)
		return
	}
	defer trayUser32.NewProc("DestroyMenu").Call(menu)
	appendItem := func(flags, id uintptr, title string) {
		trayUser32.NewProc("AppendMenuW").Call(menu, flags, id, uintptr(unsafe.Pointer(trayText(title))))
	}
	appendItem(0, 1, "Open inspector")
	state, startupErr := t.startup.Get()
	enabled := state.StartAtLogin
	var startupFlags uintptr
	if enabled {
		startupFlags = 8
	} // MF_CHECKED
	if startupErr != nil {
		startupFlags |= 1
	} // MF_GRAYED
	appendItem(startupFlags, 2, "Start at sign-in")
	appendItem(0x0800, 0, "") // MF_SEPARATOR
	appendItem(0, 3, "Quit HttpStackLens")
	var point trayPoint
	trayUser32.NewProc("GetCursorPos").Call(uintptr(unsafe.Pointer(&point)))
	trayUser32.NewProc("SetForegroundWindow").Call(t.window)
	command, _, _ := trayUser32.NewProc("TrackPopupMenu").Call(menu, 0x0102, uintptr(int64(point.X)), uintptr(int64(point.Y)), 0, t.window, 0)
	trayUser32.NewProc("PostMessageW").Call(t.window, 0, 0, 0)
	switch command {
	case 1:
		go t.show()
	case 2:
		if _, err := t.startup.SetEnabled(!enabled); err != nil {
			slog.Error("Could not change startup registration", "error", err)
			trayUser32.NewProc("MessageBoxW").Call(t.window, uintptr(unsafe.Pointer(trayText("Could not change start at sign-in:\n"+err.Error()))), uintptr(unsafe.Pointer(trayText("HttpStackLens"))), 0x10)
		}
	case 3:
		go t.quit()
	}
}
