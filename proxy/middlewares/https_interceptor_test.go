package middlewares

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

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
