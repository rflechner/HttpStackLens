package storage

import "sync"

// CaptureWriterFactory opens a fresh capture session (a new .capture file with
// its header written). It is called every time storage is switched on so each
// storage-on period produces its own self-contained capture file.
type CaptureWriterFactory func() (CaptureSessionWriter, error)

// StorageSink is a runtime-switchable CaptureSessionWriter. Enabling it opens a
// new capture session through the factory; disabling it closes the current one.
// Writes while disabled are silently dropped, so it can be wired into the proxy
// pipeline once at startup and toggled on/off without recreating the pipeline.
//
// It is independent from CaptureController (which gates the live UI view): the
// sink governs only what is persisted to disk.
type StorageSink struct {
	mu      sync.RWMutex
	factory CaptureWriterFactory
	current CaptureSessionWriter
}

func NewStorageSink(factory CaptureWriterFactory) *StorageSink {
	return &StorageSink{factory: factory}
}

// Enabled reports whether a capture session is currently open (i.e. traffic is
// being persisted to disk).
func (s *StorageSink) Enabled() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current != nil
}

// Enable opens a new capture session if none is open. It is a no-op when storage
// is already on, so repeated enables do not spawn extra files.
func (s *StorageSink) Enable() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current != nil {
		return nil
	}
	w, err := s.factory()
	if err != nil {
		return err
	}
	s.current = w
	return nil
}

// Disable closes the current capture session, if any. It is a no-op when storage
// is already off.
func (s *StorageSink) Disable() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closeCurrentLocked()
}

func (s *StorageSink) WriteRequest(r RequestRecord) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.current == nil {
		return nil
	}
	return s.current.WriteRequest(r)
}

func (s *StorageSink) WriteResponse(r ResponseRecord) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.current == nil {
		return nil
	}
	return s.current.WriteResponse(r)
}

func (s *StorageSink) Flush() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.current == nil {
		return nil
	}
	return s.current.Flush()
}

// Close closes the current capture session and leaves the sink disabled. It is
// safe to call at shutdown whether or not storage is currently on.
func (s *StorageSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closeCurrentLocked()
}

func (s *StorageSink) closeCurrentLocked() error {
	if s.current == nil {
		return nil
	}
	err := s.current.Close()
	s.current = nil
	return err
}
