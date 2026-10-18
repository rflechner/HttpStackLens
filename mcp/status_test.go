package mcp

import (
	"context"
	"encoding/json"
	"httpStackLens/configuration"
	"httpStackLens/storage"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type discardCaptureWriter struct{}

func (discardCaptureWriter) WriteRequest(storage.RequestRecord) error   { return nil }
func (discardCaptureWriter) WriteResponse(storage.ResponseRecord) error { return nil }
func (discardCaptureWriter) Flush() error                               { return nil }
func (discardCaptureWriter) Close() error                               { return nil }

func connectStatusClient(t *testing.T, deps Dependencies) (context.Context, *sdk.ClientSession) {
	t.Helper()
	server := httptest.NewServer(newHandler(deps))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	client := sdk.NewClient(&sdk.Implementation{Name: "status-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &sdk.StreamableClientTransport{Endpoint: server.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return ctx, session
}

func TestGetStatusLiveAndSanitized(t *testing.T) {
	deps := Dependencies{
		Proxy:        storage.NewProxyController(true),
		Capture:      storage.NewCaptureController(true),
		Storage:      storage.NewStorageSink(func() (storage.CaptureSessionWriter, error) { return discardCaptureWriter{}, nil }),
		Requests:     storage.NewRequestStore(10),
		DecryptHTTPS: configuration.NewDecryptHttpsConfigStore(configuration.DecryptHttpsConfig{}),
		Upstream:     configuration.NewUpstreamSettingsStore(configuration.UpstreamSettings{}),
	}
	t.Cleanup(func() { _ = deps.Storage.Close() })
	deps.Proxy.SetAddress("127.0.0.1:3128")
	ctx, session := connectStatusClient(t, deps)
	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tool := range listed.Tools {
		if tool.Name == "get_status" {
			found = true
			if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint || tool.OutputSchema == nil {
				t.Fatal("missing read-only annotation or output schema")
			}
		}
	}
	if !found {
		t.Fatal("get_status not advertised")
	}
	call := func() Status {
		t.Helper()
		result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: "get_status", Arguments: map[string]any{}})
		if err != nil {
			t.Fatal(err)
		}
		if result.IsError {
			t.Fatalf("tool error: %+v", result)
		}
		data, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		all, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{"secret-password", "private-proxy.example", "secret-token"} {
			if strings.Contains(string(all), secret) {
				t.Fatalf("status leaked %s", secret)
			}
		}
		var status Status
		if err := json.Unmarshal(data, &status); err != nil {
			t.Fatal(err)
		}
		if len(result.Content) == 0 {
			t.Fatal("missing text fallback for clients")
		}
		return status
	}
	initial := call()
	if !initial.Proxy.Running || initial.Proxy.Address != "127.0.0.1:3128" || !initial.Capture.Recording || initial.Capture.PersistenceEnabled || initial.Upstream.Enabled || initial.HTTPS.DecryptionEnabled {
		t.Fatalf("unexpected initial status: %+v", initial)
	}
	deps.Proxy.SetRunning(false)
	deps.Capture.Pause()
	if err := deps.Storage.Enable(); err != nil {
		t.Fatal(err)
	}
	deps.Requests.PutRequest("secret-token", storage.RequestRecord{})
	deps.DecryptHTTPS.UpdateEnabled(true)
	deps.Upstream.Update(configuration.UpstreamSettings{OutputProxyUri: "http://user:secret-password@private-proxy.example:8080/?token=secret-token", AddWindowsAuthentication: true})
	updated := call()
	if updated.Proxy.Running || updated.Capture.Recording || !updated.Capture.PersistenceEnabled || updated.Capture.BufferedExchanges != 1 || !updated.HTTPS.DecryptionEnabled || !updated.Upstream.Enabled || !updated.Upstream.WindowsAuthenticationConfigured || updated.Upstream.WindowsAuthenticationApplicable != (runtime.GOOS == "windows") {
		t.Fatalf("stale or incorrect status: %+v", updated)
	}
}

func TestGetStatusUnavailable(t *testing.T) {
	ctx, session := connectStatusClient(t, Dependencies{})
	result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: "get_status", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatalf("missing dependencies must not report a healthy status: %+v", result)
	}
}
