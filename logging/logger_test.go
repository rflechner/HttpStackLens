package logging

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMultiHandler_FansOutToEverySink(t *testing.T) {
	var sinkA, sinkB bytes.Buffer
	logger := slog.New(NewMultiHandler(
		slog.NewTextHandler(&sinkA, nil),
		slog.NewJSONHandler(&sinkB, nil),
	))

	logger.Info("hello", "key", "value")

	if !strings.Contains(sinkA.String(), "hello") {
		t.Errorf("text sink did not receive the record: %q", sinkA.String())
	}
	if !strings.Contains(sinkB.String(), "hello") || !strings.Contains(sinkB.String(), `"key":"value"`) {
		t.Errorf("json sink did not receive the record: %q", sinkB.String())
	}
}

func TestMultiHandler_RespectsPerSinkLevel(t *testing.T) {
	var quiet, verbose bytes.Buffer
	logger := slog.New(NewMultiHandler(
		slog.NewTextHandler(&quiet, &slog.HandlerOptions{Level: slog.LevelWarn}),
		slog.NewTextHandler(&verbose, &slog.HandlerOptions{Level: slog.LevelDebug}),
	))

	logger.Debug("debug-only message")

	if quiet.Len() != 0 {
		t.Errorf("warn-level sink should have filtered the debug record, got: %q", quiet.String())
	}
	if !strings.Contains(verbose.String(), "debug-only message") {
		t.Errorf("debug-level sink should have received the record, got: %q", verbose.String())
	}
}

func TestMultiHandler_EnabledIfAnySinkEnabled(t *testing.T) {
	h := NewMultiHandler(
		slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelError}),
		slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelDebug}),
	)
	if !h.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("expected Enabled to be true when at least one sink accepts the level")
	}
}

func TestParseLevel(t *testing.T) {
	cases := map[string]slog.Level{
		"debug":   slog.LevelDebug,
		"INFO":    slog.LevelInfo,
		"warn":    slog.LevelWarn,
		"warning": slog.LevelWarn,
		"error":   slog.LevelError,
		"":        slog.LevelInfo,
		"nonsense": slog.LevelInfo,
	}
	for input, want := range cases {
		if got := ParseLevel(input); got != want {
			t.Errorf("ParseLevel(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestSetup_CreatesFileSink(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "nested", "app.log")

	cleanup, err := Setup(slog.LevelInfo, logPath)
	if err != nil {
		t.Fatalf("Setup returned error: %v", err)
	}
	defer cleanup()

	slog.Info("written to file", "answer", 42)

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("expected log file to exist: %v", err)
	}
	if !strings.Contains(string(data), "written to file") {
		t.Errorf("log file missing the record: %q", string(data))
	}
}
