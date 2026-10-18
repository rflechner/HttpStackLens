package mcp

import (
	"context"
	"fmt"
	"httpStackLens/composer"
	"httpStackLens/configuration"
	"httpStackLens/storage"
	"httpStackLens/webui/wasm/shared"
	"runtime"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Dependencies are the same live stores used by the Web UI. No configuration
// or captured traffic is serialized directly into MCP results.
type Dependencies struct {
	Proxy        *storage.ProxyController
	Capture      *storage.CaptureController
	Storage      *storage.StorageSink
	Requests     *storage.RequestStore
	DecryptHTTPS *configuration.DecryptHttpsConfigStore
	Upstream     *configuration.UpstreamSettingsStore
	// HttpFiles and SendComposer back the composer tools: the same .http
	// collection and pipeline sender as the Web UI composer.
	HttpFiles    *composer.HttpFileStore
	SendComposer ComposerSender
	// ComposerFileChanged is told about every file the composer tools write, so
	// that the Web UI can refresh an open composer. Optional.
	ComposerFileChanged func(shared.HttpFileChangedDto)
	// Certificate reads the local root CA, so that the instructions for
	// enabling HTTPS decryption match what the user will see. WebUIURL is
	// where those instructions send the user. Both optional.
	Certificate func() CertificateState
	WebUIURL    string
	// StateChanged and Called report the listener state and each traced call,
	// so that the Web UI MCP tab follows them live. Optional.
	StateChanged func(shared.McpStateDto)
	Called       func(shared.McpCallDto)
}

type Status struct {
	UserAction *UserAction    `json:"user_action,omitempty" jsonschema:"When present, guide the user through its steps: it cannot be done through MCP"`
	Proxy      ProxyStatus    `json:"proxy"`
	Capture    CaptureStatus  `json:"capture"`
	HTTPS      HTTPSStatus    `json:"https"`
	Upstream   UpstreamStatus `json:"upstream"`
}

type ProxyStatus struct {
	Running bool   `json:"running" jsonschema:"Whether the proxy listener accepts new connections; stopping it does not disable Composer sends"`
	Address string `json:"address" jsonschema:"Proxy listener address"`
}

type CaptureStatus struct {
	Recording          bool `json:"recording" jsonschema:"Whether new traffic is recorded; independent of proxy forwarding"`
	PersistenceEnabled bool `json:"persistence_enabled" jsonschema:"Whether a capture file is open; recording must also be enabled to capture new traffic"`
	BufferedExchanges  int  `json:"buffered_exchanges" jsonschema:"Number of exchanges currently retained in memory, not a lifetime total"`
}

type HTTPSStatus struct {
	DecryptionEnabled bool `json:"decryption_enabled" jsonschema:"MITM setting for new CONNECT requests; existing tunnels retain their original mode and capture rules may omit bodies. Only the user can change it, from the Web UI"`
}

type UpstreamStatus struct {
	Enabled                         bool `json:"enabled"`
	WindowsAuthenticationConfigured bool `json:"windows_authentication_configured"`
	WindowsAuthenticationApplicable bool `json:"windows_authentication_applicable" jsonschema:"Configured with an upstream on Windows; does not indicate successful authentication"`
}

func (d Dependencies) getStatus(context.Context, *sdk.CallToolRequest, struct{}) (*sdk.CallToolResult, Status, error) {
	if d.Proxy == nil || d.Capture == nil || d.Storage == nil || d.Requests == nil || d.DecryptHTTPS == nil || d.Upstream == nil {
		return nil, Status{}, fmt.Errorf("runtime status unavailable: MCP dependencies are not configured")
	}
	up := d.Upstream.Get()
	enabled := strings.TrimSpace(up.OutputProxyUri) != ""
	status := Status{
		Proxy:    ProxyStatus{Running: d.Proxy.IsRunning(), Address: d.Proxy.Address()},
		Capture:  CaptureStatus{Recording: d.Capture.IsCapturing(), PersistenceEnabled: d.Storage.Enabled(), BufferedExchanges: d.Requests.Len()},
		HTTPS:    HTTPSStatus{DecryptionEnabled: d.DecryptHTTPS.Get().Enabled},
		Upstream: UpstreamStatus{Enabled: enabled, WindowsAuthenticationConfigured: up.AddWindowsAuthentication, WindowsAuthenticationApplicable: enabled && up.AddWindowsAuthentication && runtime.GOOS == "windows"},
	}
	status.UserAction = d.decryptionUserAction()
	return nil, status, nil
}
