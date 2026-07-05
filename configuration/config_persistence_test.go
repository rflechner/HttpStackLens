package configuration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPersistStorageEnabledUpdatesExistingValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	input := "proxy:\n  port: 3128\n\nstorage:\n  enable: false # keep me\n  folder: \"captures\"\n"
	if err := os.WriteFile(path, []byte(input), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if err := persistStorageEnabled(path, true); err != nil {
		t.Fatalf("persistStorageEnabled: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "  enable: true # keep me") {
		t.Fatalf("updated config = %q", got)
	}
	if !strings.Contains(got, "  folder: \"captures\"") {
		t.Fatalf("folder was not preserved: %q", got)
	}
}

func TestPersistStorageEnabledReturnsErrorWhenMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("storage:\n  folder: captures\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if err := persistStorageEnabled(path, true); err == nil {
		t.Fatal("expected missing storage.enable error")
	}
}

func TestPersistDecryptHttpsCaptureRulesReplacesRulesOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	input := `proxy:
  port: 3128

decrypt_https:
  enabled: false # keep toggle
  cert_manager:
    ca_cert_file: "ca.crt"
  default_max_bytes: 100
  mime_types:
    - name: "image/*"
      max_size_mb: 2.5

webui:
  port: 9000
`
	if err := os.WriteFile(path, []byte(input), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	defaultMax := int64(2048)
	jsonLimit := int64(512)
	if err := persistDecryptHttpsCaptureRules(path, DecryptHttpsConfig{
		DefaultMaxBytes: &defaultMax,
		MimeTypes: []MimeTypeRule{
			{Name: "application/json", MaxSizeBytes: &jsonLimit},
			{Name: "text/*"},
		},
	}); err != nil {
		t.Fatalf("persistDecryptHttpsCaptureRules: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	got := string(data)
	for _, want := range []string{
		"  enabled: false # keep toggle",
		"  cert_manager:\n    ca_cert_file: \"ca.crt\"",
		"  default_max_bytes: 2048",
		"    - name: \"application/json\"\n      max_size_bytes: 512",
		"    - name: \"text/*\"",
		"webui:\n  port: 9000",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("updated config missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "image/*") || strings.Contains(got, "max_size_mb: 2.5") {
		t.Fatalf("old rules still present:\n%s", got)
	}
}

func TestPersistDecryptHttpsCaptureRulesReturnsErrorWhenMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("proxy:\n  port: 3128\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if err := persistDecryptHttpsCaptureRules(path, DecryptHttpsConfig{}); err == nil {
		t.Fatal("expected missing decrypt_https error")
	}
}
