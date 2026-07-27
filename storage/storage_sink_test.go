package storage

import (
	"errors"
	"testing"
)

// countingWriter is a no-op CaptureSessionWriter that records how many records it
// received and whether it was closed, so tests can assert routing and lifecycle.
type countingWriter struct {
	requests  int
	responses int
	closed    bool
}

func (w *countingWriter) WriteRequest(RequestRecord) error   { w.requests++; return nil }
func (w *countingWriter) WriteResponse(ResponseRecord) error { w.responses++; return nil }
func (w *countingWriter) Flush() error                       { return nil }
func (w *countingWriter) Close() error                       { w.closed = true; return nil }

func TestStorageSinkDropsWritesWhenDisabled(t *testing.T) {
	var opened int
	sink := NewStorageSink(func() (CaptureSessionWriter, error) {
		opened++
		return &countingWriter{}, nil
	})

	if sink.Enabled() {
		t.Fatal("new sink should be disabled")
	}
	// Writes while disabled are dropped without opening a file.
	if err := sink.WriteRequest(RequestRecord{}); err != nil {
		t.Fatalf("WriteRequest while disabled: %v", err)
	}
	if opened != 0 {
		t.Fatalf("factory called %d times while disabled, want 0", opened)
	}
}

func TestStorageSinkEnableOpensAndRoutes(t *testing.T) {
	var current *countingWriter
	sink := NewStorageSink(func() (CaptureSessionWriter, error) {
		current = &countingWriter{}
		return current, nil
	})

	if err := sink.Enable(); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	if !sink.Enabled() {
		t.Fatal("sink should be enabled after Enable")
	}
	// A second Enable is a no-op: it must not open a new file.
	first := current
	if err := sink.Enable(); err != nil {
		t.Fatalf("second Enable: %v", err)
	}
	if current != first {
		t.Fatal("second Enable opened a new writer, want no-op")
	}

	_ = sink.WriteRequest(RequestRecord{})
	_ = sink.WriteResponse(ResponseRecord{})
	if current.requests != 1 || current.responses != 1 {
		t.Fatalf("routed requests=%d responses=%d, want 1/1", current.requests, current.responses)
	}
}

func TestStorageSinkDisableClosesAndReopensFreshFile(t *testing.T) {
	var writers []*countingWriter
	sink := NewStorageSink(func() (CaptureSessionWriter, error) {
		w := &countingWriter{}
		writers = append(writers, w)
		return w, nil
	})

	if err := sink.Enable(); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	if err := sink.Disable(); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	if sink.Enabled() {
		t.Fatal("sink should be disabled after Disable")
	}
	if !writers[0].closed {
		t.Fatal("Disable must close the current writer")
	}

	// Re-enabling opens a brand-new session (one file per storage-on period).
	if err := sink.Enable(); err != nil {
		t.Fatalf("re-Enable: %v", err)
	}
	if len(writers) != 2 {
		t.Fatalf("opened %d writers, want 2 (one per enable)", len(writers))
	}
}

func TestStorageSinkEnablePropagatesFactoryError(t *testing.T) {
	sink := NewStorageSink(func() (CaptureSessionWriter, error) {
		return nil, errors.New("disk full")
	})
	if err := sink.Enable(); err == nil {
		t.Fatal("Enable should surface the factory error")
	}
	if sink.Enabled() {
		t.Fatal("sink must stay disabled when the factory fails")
	}
}
