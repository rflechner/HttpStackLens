package webui

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	"httpStackLens/webui/wasm/shared"
)

type desktopSettingsService interface {
	Get() (shared.DesktopSettingsDto, error)
	SetEnabled(bool) (shared.DesktopSettingsDto, error)
}

func desktopSettingsHandler(settings desktopSettingsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPut {
			w.Header().Set("Allow", "GET, PUT")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if settings == nil {
			http.Error(w, "desktop settings unavailable", http.StatusServiceUnavailable)
			return
		}
		state, err := settings.Get()
		if err != nil {
			http.Error(w, "could not read Windows startup registration", http.StatusInternalServerError)
			return
		}
		if r.Method == http.MethodPut {
			var dto struct {
				StartAtLogin *bool `json:"start_at_login"`
			}
			decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&dto); err != nil || dto.StartAtLogin == nil {
				http.Error(w, "start_at_login must be a boolean", http.StatusBadRequest)
				return
			}
			if err := decoder.Decode(new(any)); err != io.EOF {
				http.Error(w, "expected one JSON object", http.StatusBadRequest)
				return
			}
			if !state.Supported {
				http.Error(w, "start at sign-in is only supported on Windows", http.StatusNotImplemented)
				return
			}
			state, err = settings.SetEnabled(*dto.StartAtLogin)
			if err != nil {
				log.Printf("Could not save desktop settings: %v", err)
				http.Error(w, "could not save desktop settings; check config.yaml write access and Windows startup registration", http.StatusInternalServerError)
				return
			}
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, state)
	}
}
