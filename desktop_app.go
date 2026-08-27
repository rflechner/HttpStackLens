package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"sync"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Wails needs an initial document before its runtime and lifecycle hooks are
// ready. The actual inspector remains on the local HTTP server because Wails'
// Windows asset protocol cannot stream SSE responses.
//
//go:embed desktop/index.html
var desktopBootstrap embed.FS

func runDesktopApp(webUiPort int, requestStop func()) error {
	assets, err := fs.Sub(desktopBootstrap, "desktop")
	if err != nil {
		return fmt.Errorf("prepare Wails bootstrap assets: %w", err)
	}

	webUiURL := fmt.Sprintf("http://127.0.0.1:%d/", webUiPort)
	encodedURL, err := json.Marshal(webUiURL)
	if err != nil {
		return fmt.Errorf("encode Web UI URL: %w", err)
	}
	var navigateOnce sync.Once

	return wails.Run(&options.App{
		Title:       "HttpStackLens",
		Width:       1440,
		Height:      900,
		MinWidth:    960,
		MinHeight:   640,
		AssetServer: &assetserver.Options{Assets: assets},
		BackgroundColour: &options.RGBA{
			R: 241,
			G: 238,
			B: 232,
			A: 255,
		},
		OnDomReady: func(ctx context.Context) {
			navigateOnce.Do(func() {
				wailsRuntime.WindowExecJS(ctx, "window.location.replace("+string(encodedURL)+")")
			})
		},
		OnShutdown: func(context.Context) {
			requestStop()
		},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "httpstacklens-desktop-app",
		},
		Windows: &windows.Options{
			Theme:                windows.SystemDefault,
			DisablePinchZoom:     true,
			IsZoomControlEnabled: true,
		},
	})
}
