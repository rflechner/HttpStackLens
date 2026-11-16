//go:build js && wasm

package main

import (
	"encoding/json"
	"syscall/js"

	"httpStackLens/webui/wasm/dom"
	"httpStackLens/webui/wasm/shared"
)

func registerDesktopSettingsBridges() {
	js.Global().Set("hslLoadDesktopSettings", js.FuncOf(func(js.Value, []js.Value) any {
		go fetchDesktopSettings("GET", "")
		return nil
	}))
	js.Global().Set("hslSaveDesktopSettings", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) == 1 && args[0].Type() == js.TypeBoolean {
			body, _ := json.Marshal(map[string]bool{"start_at_login": args[0].Bool()})
			go fetchDesktopSettings("PUT", string(body))
		}
		return nil
	}))
}

func fetchDesktopSettings(method, body string) {
	response, err := dom.Fetch(method, "/api/settings/desktop", map[string]string{"Content-Type": "application/json"}, body)
	if err != nil {
		callMockup("setDesktopSettings", map[string]any{"error": "Could not contact the application: " + err.Error()})
		return
	}
	if response.Status != 200 {
		callMockup("setDesktopSettings", map[string]any{"error": response.Body})
		return
	}
	var dto shared.DesktopSettingsDto
	if err := json.Unmarshal([]byte(response.Body), &dto); err != nil {
		callMockup("setDesktopSettings", map[string]any{"error": "Could not read desktop settings."})
		return
	}
	callMockup("setDesktopSettings", map[string]any{
		"supported": dto.Supported, "startAtLogin": dto.StartAtLogin,
		"registered": dto.Registered, "startedInBackground": dto.StartedInBackground,
		"saved": method == "PUT",
	})
}
