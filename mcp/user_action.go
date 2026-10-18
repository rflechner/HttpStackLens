package mcp

import "strings"

// UserAction is something only the user can do, returned for the model to
// explain step by step. It is never something the model can do itself.
type UserAction struct {
	Reason string   `json:"reason"`
	Steps  []string `json:"steps" jsonschema:"Instructions to give the user, in order"`
	Notes  []string `json:"notes,omitempty"`
	Verify string   `json:"verify" jsonschema:"How the model checks, once the user says it is done"`
}

// CertificateState is the local root CA as far as HTTPS decryption needs it.
// Known is false when the state could not be read; the instructions then
// cover every case.
type CertificateState struct {
	Known            bool
	Available        bool // a CA certificate and key were loaded
	Expired          bool
	Installed        bool // trusted by the operating system
	InstallSupported bool // HttpStackLens can install it in the OS trust store
}

const decryptionOffReason = "HTTPS decryption is off, so HTTPS traffic is only visible as CONNECT tunnels: host and port, without path, status, headers or timing. " +
	"It cannot be enabled through MCP: it makes this machine trust a local certificate authority able to read the user's HTTPS traffic, which only the user may decide. " +
	"Explain the steps below to the user."

// decryptionUserAction returns the instructions to relay when HTTPS
// decryption is off, or nil when it is on or its state is unknown.
func (d Dependencies) decryptionUserAction() *UserAction {
	if d.DecryptHTTPS == nil || d.DecryptHTTPS.Get().Enabled {
		return nil
	}
	ca := CertificateState{}
	if d.Certificate != nil {
		ca = d.Certificate()
	}
	return enableDecryptionAction(d.WebUIURL, ca)
}

// enableDecryptionAction follows the Web UI as it is: the toolbar button opens
// a setup window that generates the CA when there is none, offers to trust
// it, and enables decryption on "Done". A browser only reads its trust store
// at start-up, hence the restart whenever a certificate was just created or
// trusted.
func enableDecryptionAction(webUIURL string, ca CertificateState) *UserAction {
	open := "Open the HttpStackLens Web UI"
	if webUIURL != "" {
		open += " at " + webUIURL
	}
	open += " and select the Traffic tab in the title bar."
	toggle := `In the toolbar, click "HTTPS decryption · Off": the "HTTPS decryption setup" window opens.`
	trust := `Click "Install in OS trust store" and accept the system prompt if one appears (it may ask for the session password).`
	if ca.Known && !ca.InstallSupported {
		trust = `Click "Export public certificate", then import that file as a trusted root certification authority in the operating system or the browser: HttpStackLens cannot install it automatically on this system.`
	}
	done := `Click "Done — enable decryption".`
	restart := "Quit the browser completely (every window, not only the tab) and start it again, so that it trusts the new certificate."
	reproduce := "Reproduce the traffic to analyse: connections opened before decryption was enabled stay opaque."

	action := &UserAction{
		Reason: decryptionOffReason,
		Notes: []string{
			"Firefox, Node.js, Java and some other tools use their own trust store instead of the system one: import the exported public certificate there as well (Web UI: Settings → TLS / Certificate → Export public certificate).",
			"The certificate private key never leaves this machine. Decryption can be switched off at any time with the same toolbar button.",
		},
		Verify: "Once the user says it is done, call get_status: https.decryption_enabled must be true. Then search the traffic again.",
	}
	switch {
	case !ca.Known:
		action.Steps = []string{open, toggle,
			`If the window offers "Generate certificate", click it: it creates the local root certificate authority.`,
			`If the certificate is not yet trusted by the operating system: ` + lowerFirst(trust),
			done,
			"If a certificate was just generated or installed: " + lowerFirst(restart),
			reproduce}
	case ca.Available && ca.Expired:
		action.Steps = []string{open,
			`Click "Settings" (top right), open "TLS / Certificate" and click "Regenerate root": the current certificate has expired.`,
			`Click "Install" in the same panel (or export the public certificate if there is no Install button), then close Settings.`,
			toggle, done, restart, reproduce}
	case !ca.Available:
		action.Steps = []string{open, toggle,
			`Click "Generate certificate": it creates the local root certificate authority.`,
			trust, done, restart, reproduce}
	case !ca.Installed:
		action.Steps = []string{open, toggle, trust, done, restart, reproduce}
	default:
		// Already trusted: only the switch is missing.
		action.Steps = []string{open, toggle, done, reproduce}
		action.Notes = append([]string{"If the browser then reports certificate errors, restart it: it may have been running since before the certificate was trusted."}, action.Notes...)
	}
	return action
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}
