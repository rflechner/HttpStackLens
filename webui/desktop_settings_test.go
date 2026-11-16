package webui

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"httpStackLens/webui/wasm/shared"
)

type fakeDesktopSettings struct {
	state shared.DesktopSettingsDto
	err   error
	calls int
}

func (f *fakeDesktopSettings) Get() (shared.DesktopSettingsDto, error) { return f.state, nil }
func (f *fakeDesktopSettings) SetEnabled(enabled bool) (shared.DesktopSettingsDto, error) {
	f.calls++
	f.state.StartAtLogin, f.state.Registered = enabled, enabled
	return f.state, f.err
}

func TestDesktopSettingsAPI(t *testing.T) {
	for _, tc := range []struct {
		method, body  string
		supported     bool
		status, calls int
	}{
		{"GET", "", true, 200, 0},
		{"PUT", `{"start_at_login":true}`, true, 200, 1},
		{"PUT", `{"start_at_login":false}`, true, 200, 1},
		{"PUT", `{}`, true, 400, 0},
		{"PUT", `{"start_at_login":null}`, true, 400, 0},
		{"PUT", `{"start_at_login":"true"}`, true, 400, 0},
		{"PUT", `{"start_at_login":true,"unexpected":1}`, true, 400, 0},
		{"PUT", `{"start_at_login":true} {}`, true, 400, 0},
		{"PUT", `{"start_at_login":true}`, false, 501, 0},
		{"GET", "", false, 200, 0},
		{"POST", "", true, 405, 0},
	} {
		service := &fakeDesktopSettings{state: shared.DesktopSettingsDto{Supported: tc.supported}}
		response := httptest.NewRecorder()
		desktopSettingsHandler(service)(response, httptest.NewRequest(tc.method, "/api/settings/desktop", strings.NewReader(tc.body)))
		if response.Code != tc.status || service.calls != tc.calls {
			t.Fatalf("%s %s: status=%d calls=%d; want %d/%d", tc.method, tc.body, response.Code, service.calls, tc.status, tc.calls)
		}
		if response.Code == 200 {
			var state shared.DesktopSettingsDto
			if err := json.Unmarshal(response.Body.Bytes(), &state); err != nil {
				t.Fatal(err)
			}
			if state.Supported != tc.supported || state.Registered != service.state.Registered {
				t.Fatalf("response = %+v", state)
			}
		}
	}
}

func TestDesktopSettingsAPIReportsSaveFailure(t *testing.T) {
	service := &fakeDesktopSettings{state: shared.DesktopSettingsDto{Supported: true}, err: errors.New("disk full")}
	response := httptest.NewRecorder()
	desktopSettingsHandler(service)(response, httptest.NewRequest("PUT", "/api/settings/desktop", strings.NewReader(`{"start_at_login":true}`)))
	if response.Code != 500 {
		t.Fatalf("status = %d", response.Code)
	}
}
