package storage

import (
	"sync"
	"time"
)

// DefaultRequestStoreSize is the number of recent exchanges kept in memory when
// no explicit size is configured.
const DefaultRequestStoreSize = 500

// CapturedExchange is one request together with its response (when one was
// received), held in memory so the Web UI can fetch full headers and bodies on
// demand without re-reading the capture file.
type CapturedExchange struct {
	CorrelationID string
	Request       *RequestRecord
	Response      *ResponseRecord
	CreatedAt     time.Time
}

// RequestStore keeps the most recent exchanges in memory, indexed by correlation
// id. It is bounded: once it holds max exchanges, inserting a new one evicts the
// oldest (FIFO). Bodies are already size-capped by the capture limits, so total
// memory stays roughly max × (request cap + response cap).
//
// Safe for concurrent use.
type RequestStore struct {
	mu      sync.RWMutex
	max     int
	queue   []string // correlation ids in insertion order, oldest first
	entries map[string]*CapturedExchange
}

// NewRequestStore returns a store bounded to max exchanges (at least 1).
func NewRequestStore(max int) *RequestStore {
	if max < 1 {
		max = 1
	}
	return &RequestStore{
		max:     max,
		entries: make(map[string]*CapturedExchange),
	}
}

// PutRequest records the request side of an exchange. If an exchange with the
// same id already exists its request is replaced; otherwise a new one is added,
// evicting the oldest exchange when the store is full.
func (s *RequestStore) PutRequest(id string, rec RequestRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := rec
	if e, ok := s.entries[id]; ok {
		e.Request = &r
		return
	}
	s.enqueue(&CapturedExchange{CorrelationID: id, Request: &r, CreatedAt: time.Now()})
}

// PutResponse attaches the response side to an existing exchange. If the request
// was never seen (e.g. it was already evicted) a response-only entry is created.
func (s *RequestStore) PutResponse(id string, rec ResponseRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := rec
	if e, ok := s.entries[id]; ok {
		e.Response = &r
		return
	}
	s.enqueue(&CapturedExchange{CorrelationID: id, Response: &r, CreatedAt: time.Now()})
}

// enqueue inserts a new exchange and evicts the oldest while over capacity. The
// caller must hold the write lock.
func (s *RequestStore) enqueue(e *CapturedExchange) {
	s.entries[e.CorrelationID] = e
	s.queue = append(s.queue, e.CorrelationID)
	for len(s.queue) > s.max {
		oldest := s.queue[0]
		s.queue = s.queue[1:]
		delete(s.entries, oldest)
	}
}

// Get returns a copy of the stored exchange for id. The copy is shallow: the
// Request/Response pointers reference immutable records, so it is safe to read
// after the lock is released.
func (s *RequestStore) Get(id string) (CapturedExchange, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.entries[id]
	if !ok {
		return CapturedExchange{}, false
	}
	return *e, true
}

// Len returns the number of exchanges currently held.
func (s *RequestStore) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.entries)
}

// Clear removes all remembered exchanges from the in-memory buffer.
func (s *RequestStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queue = nil
	s.entries = make(map[string]*CapturedExchange)
}
