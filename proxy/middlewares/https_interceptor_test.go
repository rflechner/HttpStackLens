package middlewares

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"
)

// TestRequestTraceTiming checks the per-phase durations are derived from the
// captured httptrace timestamps, and that missing/out-of-order phases collapse
// to zero (as on a reused keep-alive connection).
func TestRequestTraceTiming(t *testing.T) {
	base := time.Now()
	at := func(ms int) time.Time { return base.Add(time.Duration(ms) * time.Millisecond) }

	rt := &requestTrace{
		dnsStart:     at(0),
		dnsDone:      at(10),
		connectStart: at(10),
		connectDone:  at(30),
		tlsStart:     at(30),
		tlsDone:      at(60),
		wroteRequest: at(60),
		firstByte:    at(100),
	}

	got := rt.timing(at(0), at(150))
	if got.Dns != 10*time.Millisecond {
		t.Errorf("Dns = %v, want 10ms", got.Dns)
	}
	if got.Connect != 20*time.Millisecond {
		t.Errorf("Connect = %v, want 20ms", got.Connect)
	}
	if got.Tls != 30*time.Millisecond {
		t.Errorf("Tls = %v, want 30ms", got.Tls)
	}
	if got.Ttfb != 40*time.Millisecond {
		t.Errorf("Ttfb = %v, want 40ms", got.Ttfb)
	}
	if got.Download != 50*time.Millisecond {
		t.Errorf("Download = %v, want 50ms", got.Download)
	}
	if got.Total != 150*time.Millisecond {
		t.Errorf("Total = %v, want 150ms", got.Total)
	}
}

// TestRequestTraceTimingReusedConnection covers a keep-alive request where no
// DNS/connect/TLS occurred: those phases must be zero, and TTFB falls back to
// the overall start when the request-written timestamp is missing.
func TestRequestTraceTimingReusedConnection(t *testing.T) {
	base := time.Now()
	rt := &requestTrace{firstByte: base.Add(20 * time.Millisecond)}

	got := rt.timing(base, base.Add(70*time.Millisecond))
	if got.Dns != 0 || got.Connect != 0 || got.Tls != 0 {
		t.Errorf("reused connection should have zero dns/connect/tls: %+v", got)
	}
	if got.Ttfb != 20*time.Millisecond {
		t.Errorf("Ttfb = %v, want 20ms (fallback to start)", got.Ttfb)
	}
	if got.Download != 50*time.Millisecond {
		t.Errorf("Download = %v, want 50ms", got.Download)
	}
}

// TestCapBodyAlwaysForwardsFullBody is the guarantee that the size limit never
// truncates what the browser receives: whether the body fits or is skipped, the
// forward reader must replay every original byte.
func TestCapBodyAlwaysForwardsFullBody(t *testing.T) {
	const body = "0123456789ABCDEF" // 16 bytes

	cases := []struct {
		name        string
		limit       int64
		wantSkipped bool
		wantStored  string // "" means nothing stored
	}{
		{"well under limit", 100, false, body},
		{"exactly at limit", 16, false, body},
		{"one over limit", 15, true, ""},
		{"zero limit excludes type", 0, true, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := strings.NewReader(body)

			store, forward, skipped, err := capBody(src, tc.limit)
			if err != nil {
				t.Fatalf("capBody: %v", err)
			}
			if skipped != tc.wantSkipped {
				t.Fatalf("skipped = %v, want %v", skipped, tc.wantSkipped)
			}
			if string(store) != tc.wantStored {
				t.Fatalf("stored = %q, want %q", store, tc.wantStored)
			}

			// The forwarded stream must reconstruct the complete body in every case.
			forwarded, err := io.ReadAll(forward)
			if err != nil {
				t.Fatalf("read forward: %v", err)
			}
			if !bytes.Equal(forwarded, []byte(body)) {
				t.Fatalf("forwarded = %q, want %q (browser would get a broken page)", forwarded, body)
			}
		})
	}
}

// TestCaptureLimitWriter verifies the streaming capture buffer keeps the same
// fit/skip semantics as capBody, regardless of how the body is chunked across
// Write calls (io.TeeReader may deliver it in arbitrary pieces).
func TestCaptureLimitWriter(t *testing.T) {
	const body = "0123456789ABCDEF" // 16 bytes

	cases := []struct {
		name        string
		limit       int64
		wantSkipped bool
		wantStored  string // "" means nothing stored
	}{
		{"well under limit", 100, false, body},
		{"exactly at limit", 16, false, body},
		{"one over limit", 15, true, ""},
		{"zero limit excludes type", 0, true, ""},
	}

	// chunk exercises single-shot writes and finer streaming, since io.TeeReader
	// may hand the body over in arbitrary pieces.
	for _, chunk := range []int{len(body), 5, 1} {
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				w := &captureLimitWriter{limit: tc.limit}
				for i := 0; i < len(body); i += chunk {
					end := i + chunk
					if end > len(body) {
						end = len(body)
					}
					n, err := w.Write([]byte(body[i:end]))
					if err != nil || n != end-i {
						t.Fatalf("Write returned (%d, %v), want (%d, nil)", n, err, end-i)
					}
				}

				stored, skipped := w.captured()
				if skipped != tc.wantSkipped {
					t.Fatalf("chunk=%d skipped = %v, want %v", chunk, skipped, tc.wantSkipped)
				}
				if string(stored) != tc.wantStored {
					t.Fatalf("chunk=%d stored = %q, want %q", chunk, stored, tc.wantStored)
				}
			})
		}
	}
}

func TestCaptureLimitWriterEmpty(t *testing.T) {
	w := &captureLimitWriter{limit: 500}
	stored, skipped := w.captured()
	if skipped {
		t.Fatalf("empty body should not be skipped")
	}
	if len(stored) != 0 {
		t.Fatalf("stored = %q, want empty", stored)
	}
}

func TestCapBodyEmpty(t *testing.T) {
	store, forward, skipped, err := capBody(strings.NewReader(""), 500)
	if err != nil {
		t.Fatal(err)
	}
	if skipped {
		t.Fatalf("empty body should not be skipped")
	}
	if len(store) != 0 {
		t.Fatalf("store = %q, want empty", store)
	}
	if got, _ := io.ReadAll(forward); len(got) != 0 {
		t.Fatalf("forwarded = %q, want empty", got)
	}
}
