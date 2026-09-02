package main

import (
	"runtime"
	"strings"
	"testing"
)

func TestResolvePlatformDefaultsToBuildHost(t *testing.T) {
	platform, targetOS, err := resolvePlatform("")
	if err != nil {
		t.Fatal(err)
	}
	if want := runtime.GOOS + "/" + runtime.GOARCH; platform != want {
		t.Fatalf("platform = %q, want %q", platform, want)
	}
	if targetOS != runtime.GOOS {
		t.Fatalf("targetOS = %q, want %q", targetOS, runtime.GOOS)
	}
}

func TestResolvePlatformUsesExplicitTarget(t *testing.T) {
	platform, targetOS, err := resolvePlatform("windows/arm64")
	if err != nil {
		t.Fatal(err)
	}
	if platform != "windows/arm64" || targetOS != "windows" {
		t.Fatalf("resolvePlatform = (%q, %q), want (windows/arm64, windows)", platform, targetOS)
	}
}

func TestResolvePlatformRejectsIncompleteTarget(t *testing.T) {
	if _, _, err := resolvePlatform("windows"); err == nil {
		t.Fatal("resolvePlatform accepted a target without an architecture")
	}
}

func TestVersionLdflagsUsesProvidedReleaseMetadata(t *testing.T) {
	flags := versionLdflags(t.TempDir(), "windows", buildMetadata{
		version: "v1.2.3",
		commit:  "abc1234",
		date:    "2026-09-02T10:00:00Z",
	})
	for _, want := range []string{
		"-s -w -H windowsgui",
		"-X main.version=v1.2.3",
		"-X main.commit=abc1234",
		"-X main.date=2026-09-02T10:00:00Z",
	} {
		if !strings.Contains(flags, want) {
			t.Errorf("versionLdflags() = %q, missing %q", flags, want)
		}
	}
}

func TestVersionLdflagsDoesNotUseWindowsSubsystemForDarwin(t *testing.T) {
	flags := versionLdflags(t.TempDir(), "darwin", buildMetadata{
		version: "v1.2.3",
		commit:  "abc1234",
		date:    "2026-09-02T10:00:00Z",
	})
	if strings.Contains(flags, "windowsgui") {
		t.Fatalf("versionLdflags() = %q, contains Windows-only linker flag", flags)
	}
}
