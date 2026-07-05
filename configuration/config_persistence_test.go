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
