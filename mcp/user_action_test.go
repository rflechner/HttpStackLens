package mcp

import (
	"context"
	"httpStackLens/configuration"
	"httpStackLens/storage"
	"strings"
	"testing"
)

func TestDecryptionOffAsksTheUser(t *testing.T) {
	store := storage.NewRequestStore(10)
	store.PutRequest("tunnel", storage.RequestRecord{Method: "CONNECT", URL: "www.youtube.com:443"})
	store.PutRequest("plain", storage.RequestRecord{Method: "GET", URL: "http://example.com/"})

	for _, tc := range []struct {
		name    string
		decrypt *configuration.DecryptHttpsConfigStore
		want    bool
	}{
		{"off", configuration.NewDecryptHttpsConfigStore(configuration.DecryptHttpsConfig{Enabled: false}), true},
		{"on", configuration.NewDecryptHttpsConfigStore(configuration.DecryptHttpsConfig{Enabled: true}), false},
		{"unknown", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deps := Dependencies{Requests: store, DecryptHTTPS: tc.decrypt, WebUIURL: "http://127.0.0.1:9000/"}
			check := func(what string, action *UserAction, want bool) {
				t.Helper()
				if (action != nil) != want {
					t.Errorf("%s user_action = %+v, want present=%v", what, action, want)
				}
				if action != nil && (!strings.Contains(action.Reason, "cannot be enabled through MCP") || !strings.Contains(action.Steps[0], "http://127.0.0.1:9000/")) {
					t.Errorf("%s user_action does not send the user to the Web UI: %+v", what, action)
				}
			}

			_, search, err := deps.searchRequests(context.Background(), nil, SearchParams{})
			if err != nil {
				t.Fatal(err)
			}
			check("search_requests", search.UserAction, tc.want)

			_, tunnel, err := deps.getRequest(context.Background(), nil, GetRequestParams{ID: "tunnel"})
			if err != nil {
				t.Fatal(err)
			}
			check("get_request on a tunnel", tunnel.UserAction, tc.want)

			// Decryption would not change anything to plain HTTP.
			_, plain, err := deps.getRequest(context.Background(), nil, GetRequestParams{ID: "plain"})
			if err != nil {
				t.Fatal(err)
			}
			check("get_request on plain HTTP", plain.UserAction, false)
		})
	}
}

// The steps follow the certificate: generated or trusted just now means a
// browser restart, already trusted means only the switch.
func TestEnableDecryptionStepsFollowTheCertificate(t *testing.T) {
	for _, tc := range []struct {
		name      string
		ca        CertificateState
		want      []string
		unwanted  []string
		wantNotes []string
	}{
		{"no certificate", CertificateState{Known: true, InstallSupported: true},
			[]string{"Generate certificate", "Install in OS trust store", "Done — enable decryption", "Quit the browser completely"}, nil, nil},
		{"not trusted", CertificateState{Known: true, Available: true, InstallSupported: true},
			[]string{"Install in OS trust store", "Done — enable decryption", "Quit the browser completely"}, []string{"Generate certificate"}, nil},
		{"no automatic install", CertificateState{Known: true, Available: true},
			[]string{"Export public certificate", "Quit the browser completely"}, []string{"Install in OS trust store"}, nil},
		{"expired", CertificateState{Known: true, Available: true, Expired: true, Installed: true, InstallSupported: true},
			[]string{"Regenerate root", "Done — enable decryption", "Quit the browser completely"}, nil, nil},
		{"ready", CertificateState{Known: true, Available: true, Installed: true, InstallSupported: true},
			[]string{"HTTPS decryption · Off", "Done — enable decryption", "Reproduce the traffic"}, []string{"Generate certificate", "Install in OS trust store", "Quit the browser"}, []string{"restart it"}},
		{"unknown", CertificateState{},
			[]string{"If the window offers \"Generate certificate\"", "If the certificate is not yet trusted", "If a certificate was just generated or installed"}, nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			action := enableDecryptionAction("", tc.ca)
			steps := strings.Join(action.Steps, "\n")
			for _, want := range tc.want {
				if !strings.Contains(steps, want) {
					t.Errorf("steps lack %q:\n%s", want, steps)
				}
			}
			for _, unwanted := range tc.unwanted {
				if strings.Contains(steps, unwanted) {
					t.Errorf("steps should not mention %q:\n%s", unwanted, steps)
				}
			}
			notes := strings.Join(action.Notes, "\n")
			for _, want := range tc.wantNotes {
				if !strings.Contains(notes, want) {
					t.Errorf("notes lack %q:\n%s", want, notes)
				}
			}
			if !strings.Contains(notes, "Firefox") || !strings.Contains(action.Verify, "get_status") {
				t.Errorf("notes or verify incomplete: %+v", action)
			}
		})
	}
}
