package storage

import (
	"sync"
	"testing"
)

func TestRequestStorePutGet(t *testing.T) {
	s := NewRequestStore(10)
	s.PutRequest("a", RequestRecord{Method: "GET", URL: "http://x/a"})
	s.PutResponse("a", ResponseRecord{StatusCode: 200})

	got, ok := s.Get("a")
	if !ok {
		t.Fatal("expected exchange 'a' to be present")
	}
	if got.Request == nil || got.Request.Method != "GET" {
		t.Errorf("request not stored correctly: %+v", got.Request)
	}
	if got.Response == nil || got.Response.StatusCode != 200 {
		t.Errorf("response not stored correctly: %+v", got.Response)
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
}

func TestRequestStoreMissing(t *testing.T) {
	s := NewRequestStore(10)
	if _, ok := s.Get("nope"); ok {
		t.Error("expected miss for unknown id")
	}
}

func TestRequestStorePutRequestReplaces(t *testing.T) {
	s := NewRequestStore(10)
	s.PutRequest("a", RequestRecord{Method: "GET"})
	s.PutRequest("a", RequestRecord{Method: "POST"})
	got, _ := s.Get("a")
	if got.Request.Method != "POST" {
		t.Errorf("Method = %q, want POST (replaced)", got.Request.Method)
	}
	if s.Len() != 1 {
		t.Errorf("Len = %d, want 1 (same id must not duplicate)", s.Len())
	}
}

func TestRequestStoreResponseOnly(t *testing.T) {
	// A response whose request was never seen (or already evicted) still lands.
	s := NewRequestStore(10)
	s.PutResponse("orphan", ResponseRecord{StatusCode: 502})
	got, ok := s.Get("orphan")
	if !ok || got.Request != nil || got.Response.StatusCode != 502 {
		t.Errorf("orphan response not stored: ok=%v exch=%+v", ok, got)
	}
}

func TestRequestStoreEvictsOldest(t *testing.T) {
	s := NewRequestStore(3)
	for _, id := range []string{"1", "2", "3", "4", "5"} {
		s.PutRequest(id, RequestRecord{URL: id})
	}
	if s.Len() != 3 {
		t.Fatalf("Len = %d, want 3 (bounded)", s.Len())
	}
	// Oldest two evicted.
	for _, gone := range []string{"1", "2"} {
		if _, ok := s.Get(gone); ok {
			t.Errorf("id %q should have been evicted", gone)
		}
	}
	// Newest three retained.
	for _, kept := range []string{"3", "4", "5"} {
		if _, ok := s.Get(kept); !ok {
			t.Errorf("id %q should be retained", kept)
		}
	}
}

func TestRequestStoreReinsertsEvictedIDAsNewest(t *testing.T) {
	s := NewRequestStore(2)
	s.PutRequest("a", RequestRecord{URL: "first"})
	s.PutRequest("b", RequestRecord{URL: "second"})
	s.PutRequest("c", RequestRecord{URL: "third"})

	if _, ok := s.Get("a"); ok {
		t.Fatal("id \"a\" should have been evicted before reinsertion")
	}

	s.PutResponse("a", ResponseRecord{StatusCode: 204})

	if s.Len() != 2 {
		t.Fatalf("Len = %d, want 2", s.Len())
	}
	if _, ok := s.Get("b"); ok {
		t.Error("id \"b\" should be evicted after reinserting \"a\"")
	}
	if _, ok := s.Get("c"); !ok {
		t.Error("id \"c\" should still be retained")
	}
	got, ok := s.Get("a")
	if !ok || got.Request != nil || got.Response == nil || got.Response.StatusCode != 204 {
		t.Errorf("reinserted response-only exchange not stored correctly: ok=%v exch=%+v", ok, got)
	}
}

func TestRequestStoreGetReturnsCopy(t *testing.T) {
	// The returned exchange must not change when the store is updated afterwards.
	s := NewRequestStore(10)
	s.PutRequest("a", RequestRecord{Method: "GET"})
	snapshot, _ := s.Get("a")
	s.PutResponse("a", ResponseRecord{StatusCode: 200})
	if snapshot.Response != nil {
		t.Error("earlier snapshot must not see the later response")
	}
}

func TestRequestStoreMinSize(t *testing.T) {
	s := NewRequestStore(0) // clamped to 1
	s.PutRequest("a", RequestRecord{})
	s.PutRequest("b", RequestRecord{})
	if s.Len() != 1 {
		t.Errorf("Len = %d, want 1 (size clamped to minimum)", s.Len())
	}
}

func TestRequestStoreClear(t *testing.T) {
	s := NewRequestStore(10)
	s.PutRequest("a", RequestRecord{Method: "GET"})
	s.PutResponse("a", ResponseRecord{StatusCode: 200})
	s.PutRequest("b", RequestRecord{Method: "POST"})

	s.Clear()

	if got := s.Len(); got != 0 {
		t.Fatalf("Len after Clear = %d, want 0", got)
	}
	if _, ok := s.Get("a"); ok {
		t.Fatal("cleared store still returned exchange a")
	}
	s.PutRequest("c", RequestRecord{Method: "PUT"})
	if got := s.Len(); got != 1 {
		t.Fatalf("Len after reinserting = %d, want 1", got)
	}
}

func TestRequestStoreConcurrent(t *testing.T) {
	// Exercise the lock under -race.
	s := NewRequestStore(100)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := string(rune('a' + n%26))
			s.PutRequest(id, RequestRecord{URL: id})
			s.PutResponse(id, ResponseRecord{StatusCode: 200})
			_, _ = s.Get(id)
			_ = s.Len()
		}(i)
	}
	wg.Wait()
}
