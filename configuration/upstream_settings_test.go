package configuration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPersistUpstreamSettingsReplacesValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	input := `proxy:
  enable_remote_connection: false
  port: 3128
  output_proxy_uri: # comment
  add_windows_authentication_to_output_proxy: false
  require_windows_authentication: true # keep me

  no_proxy:
    - "localhost"
    - "127.0.0.1"

webui:
  port: 9000
`
	if err := os.WriteFile(path, []byte(input), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if err := persistUpstreamSettings(path, UpstreamSettings{
		OutputProxyUri:           "http://proxy:8080",
		NoProxy:                  []string{"example.com", ".local"},
		AddWindowsAuthentication: true,
	}); err != nil {
		t.Fatalf("persistUpstreamSettings: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	got := string(data)
	for _, want := range []string{
		"  output_proxy_uri: \"http://proxy:8080\"",
		"  add_windows_authentication_to_output_proxy: true",
		"  require_windows_authentication: true # keep me",
		"  no_proxy:\n    - \"example.com\"\n    - \".local\"",
		"webui:\n  port: 9000",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("updated config missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "localhost") || strings.Contains(got, "127.0.0.1") {
		t.Fatalf("old no_proxy entries still present:\n%s", got)
	}

	// The rewritten file must still parse and reflect the new settings.
	conf := readConfigFromPath(t, path)
	if conf.Proxy.OutputProxyUri != "http://proxy:8080" {
		t.Fatalf("output_proxy_uri = %q", conf.Proxy.OutputProxyUri)
	}
	if !conf.Proxy.AddWindowsAuthenticationToOutputProxy {
		t.Fatalf("add_windows_authentication_to_output_proxy not persisted")
	}
	if got, want := strings.Join(conf.Proxy.NoProxy, ","), "example.com,.local"; got != want {
		t.Fatalf("no_proxy = %q, want %q", got, want)
	}
	if !conf.Proxy.RequireWindowsAuthentication {
		t.Fatalf("require_windows_authentication was not preserved")
	}
}

func TestPersistUpstreamSettingsEmptyValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	input := `proxy:
  port: 3128
  output_proxy_uri: "http://old:9"
  no_proxy:
    - "localhost"
`
	if err := os.WriteFile(path, []byte(input), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if err := persistUpstreamSettings(path, UpstreamSettings{}); err != nil {
		t.Fatalf("persistUpstreamSettings: %v", err)
	}

	conf := readConfigFromPath(t, path)
	if conf.Proxy.OutputProxyUri != "" {
		t.Fatalf("output_proxy_uri = %q, want empty", conf.Proxy.OutputProxyUri)
	}
	if len(conf.Proxy.NoProxy) != 0 {
		t.Fatalf("no_proxy = %v, want empty", conf.Proxy.NoProxy)
	}
}

func TestPersistUpstreamSettingsReturnsErrorWhenMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("webui:\n  port: 9000\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if err := persistUpstreamSettings(path, UpstreamSettings{}); err == nil {
		t.Fatal("expected missing proxy error")
	}
}

func TestUpstreamSettingsStoreUpdate(t *testing.T) {
	store := NewUpstreamSettingsStore(UpstreamSettings{OutputProxyUri: "http://a"})
	updated := store.Update(UpstreamSettings{OutputProxyUri: " http://b ", NoProxy: []string{"x"}})
	if updated.OutputProxyUri != "http://b" {
		t.Fatalf("OutputProxyUri = %q, want trimmed http://b", updated.OutputProxyUri)
	}
	if got := store.Get().OutputProxyUri; got != "http://b" {
		t.Fatalf("stored OutputProxyUri = %q", got)
	}
	// Mutating the returned slice must not affect the store.
	updated.NoProxy[0] = "mutated"
	if store.Get().NoProxy[0] != "x" {
		t.Fatal("store shares the underlying slice")
	}
}

func readConfigFromPath(t *testing.T, path string) AppConfig {
	t.Helper()
	dir := filepath.Dir(path)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	return ReadConfiguration()
}
