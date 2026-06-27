package storage

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// captureReader decodes a .capture file exactly the way a real reader would, so
// the tests assert against the documented on-disk layout rather than against the
// writer's own code.
type captureReader struct {
	t   *testing.T
	b   []byte
	pos int
}

func (r *captureReader) take(n int) []byte {
	r.t.Helper()
	if r.pos+n > len(r.b) {
		r.t.Fatalf("unexpected EOF: want %d bytes at offset %d, have %d", n, r.pos, len(r.b)-r.pos)
	}
	out := r.b[r.pos : r.pos+n]
	r.pos += n
	return out
}

func (r *captureReader) byteVal() byte     { return r.take(1)[0] }
func (r *captureReader) boolVal() bool     { return r.take(1)[0] != 0 }
func (r *captureReader) int16Val() int16   { return int16(binary.LittleEndian.Uint16(r.take(2))) }
func (r *captureReader) int32Val() int32   { return int32(binary.LittleEndian.Uint32(r.take(4))) }
func (r *captureReader) int64Val() int64   { return int64(binary.LittleEndian.Uint64(r.take(8))) }
func (r *captureReader) uint32Val() uint32 { return binary.LittleEndian.Uint32(r.take(4)) }

func (r *captureReader) lpstring() string {
	n := binary.LittleEndian.Uint32(r.take(4))
	return string(r.take(int(n)))
}

func (r *captureReader) headers() []Header {
	n := r.int32Val()
	if n == 0 {
		return nil
	}
	out := make([]Header, 0, n)
	for i := int32(0); i < n; i++ {
		out = append(out, Header{Name: r.lpstring(), Value: r.lpstring()})
	}
	return out
}

func (r *captureReader) blob() []byte {
	n := r.int64Val()
	if n < 0 {
		return nil
	}
	return append([]byte(nil), r.take(int(n))...)
}

// recordTrailer validates the CRC32-C covering bytes [start:r.pos] and advances
// past the 4-byte checksum.
func (r *captureReader) recordTrailer(start int) {
	r.t.Helper()
	payload := r.b[start:r.pos]
	got := r.uint32Val()
	if want := recordChecksum(payload); got != want {
		r.t.Fatalf("record crc mismatch: got %#x want %#x", got, want)
	}
}

func writeSampleCapture(t *testing.T, httpsDecrypted bool) (string, RequestRecord, ResponseRecord) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "session.capture")

	w, err := NewFileCaptureSessionWriter(path, httpsDecrypted)
	if err != nil {
		t.Fatalf("NewFileCaptureSessionWriter: %v", err)
	}

	req := RequestRecord{
		RequestID:   UUID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		Method:      "GET",
		URL:         "https://example.com/index.html",
		HttpVersion: HttpVersion11,
		Headers: []Header{
			{Name: "Host", Value: "example.com"},
			{Name: "Accept", Value: "text/html"},
		},
		Body: nil, // absent
	}
	resp := ResponseRecord{
		RequestID:     req.RequestID,
		HttpVersion:   HttpVersion11,
		StatusCode:    200,
		StatusMessage: "OK",
		Headers:       []Header{{Name: "Content-Type", Value: "text/html"}},
		Body:          []byte("<html>hi</html>"),
	}

	if err := w.WriteRequest(req); err != nil {
		t.Fatalf("WriteRequest: %v", err)
	}
	if err := w.WriteResponse(resp); err != nil {
		t.Fatalf("WriteResponse: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return path, req, resp
}

func TestCaptureSessionRoundTrip(t *testing.T) {
	path, req, resp := writeSampleCapture(t, true)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	r := &captureReader{t: t, b: data}

	// --- header ---
	if magic := string(r.take(4)); magic != "HSLC" {
		t.Fatalf("magic = %q, want HSLC", magic)
	}
	if v := r.int16Val(); v != CaptureFormatVersion {
		t.Fatalf("version = %d, want %d", v, CaptureFormatVersion)
	}
	if !r.boolVal() {
		t.Fatalf("https_decrypted = false, want true")
	}
	if n := r.int32Val(); n != -1 {
		t.Fatalf("records_count = %d, want -1", n)
	}

	// --- request record ---
	start := r.pos
	if rt := RecordType(r.byteVal()); rt != RecordTypeRequest {
		t.Fatalf("record type = %#x, want request", rt)
	}
	var gotID UUID
	copy(gotID[:], r.take(16))
	if gotID != req.RequestID {
		t.Fatalf("request id = %v, want %v", gotID, req.RequestID)
	}
	if m := r.lpstring(); m != req.Method {
		t.Fatalf("method = %q, want %q", m, req.Method)
	}
	if u := r.lpstring(); u != req.URL {
		t.Fatalf("url = %q, want %q", u, req.URL)
	}
	if hv := HttpVersion(r.byteVal()); hv != req.HttpVersion {
		t.Fatalf("http version = %#x, want %#x", hv, req.HttpVersion)
	}
	if h := r.headers(); !headersEqual(h, req.Headers) {
		t.Fatalf("request headers = %v, want %v", h, req.Headers)
	}
	if body := r.blob(); body != nil {
		t.Fatalf("request body = %v, want nil", body)
	}
	r.recordTrailer(start)

	// --- response record ---
	start = r.pos
	if rt := RecordType(r.byteVal()); rt != RecordTypeResponse {
		t.Fatalf("record type = %#x, want response", rt)
	}
	copy(gotID[:], r.take(16))
	if gotID != resp.RequestID {
		t.Fatalf("response request id = %v, want %v", gotID, resp.RequestID)
	}
	if hv := HttpVersion(r.byteVal()); hv != resp.HttpVersion {
		t.Fatalf("http version = %#x, want %#x", hv, resp.HttpVersion)
	}
	if sc := r.int16Val(); sc != resp.StatusCode {
		t.Fatalf("status code = %d, want %d", sc, resp.StatusCode)
	}
	if sm := r.lpstring(); sm != resp.StatusMessage {
		t.Fatalf("status message = %q, want %q", sm, resp.StatusMessage)
	}
	if h := r.headers(); !headersEqual(h, resp.Headers) {
		t.Fatalf("response headers = %v, want %v", h, resp.Headers)
	}
	if body := r.blob(); string(body) != string(resp.Body) {
		t.Fatalf("response body = %q, want %q", body, resp.Body)
	}
	r.recordTrailer(start)

	if r.pos != len(data) {
		t.Fatalf("trailing bytes: consumed %d of %d", r.pos, len(data))
	}
}

func TestCaptureCorruptionIsDetected(t *testing.T) {
	path, _, _ := writeSampleCapture(t, false)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	// Flip a byte inside the first record's payload (just past the 11-byte
	// header + record-type byte) and confirm the stored CRC no longer matches.
	r := &captureReader{t: t, b: data}
	r.take(11) // skip header
	start := r.pos
	r.byteVal()  // record type
	r.take(16)   // request id
	r.lpstring() // method
	r.lpstring() // url
	r.byteVal()  // http version
	r.headers()
	r.blob()
	end := r.pos // payload is data[start:end]
	storedCRC := binary.LittleEndian.Uint32(data[end : end+4])

	// The stored CRC matches the intact payload...
	if recordChecksum(data[start:end]) != storedCRC {
		t.Fatalf("stored crc does not match intact payload")
	}
	// ...and stops matching once a payload byte is flipped.
	corrupted := append([]byte(nil), data...)
	corrupted[start+1] ^= 0xFF // damage the request id
	if recordChecksum(corrupted[start:end]) == storedCRC {
		t.Fatalf("expected crc to change after corruption")
	}
}

func TestHttpVersionEncoding(t *testing.T) {
	cases := []struct {
		major, minor int
		want         HttpVersion
	}{
		{1, 0, HttpVersion10},
		{1, 1, HttpVersion11},
		{2, 0, HttpVersion20},
		{3, 0, HttpVersion30},
	}
	for _, c := range cases {
		got := NewHttpVersion(c.major, c.minor)
		if got != c.want {
			t.Fatalf("NewHttpVersion(%d,%d) = %#x, want %#x", c.major, c.minor, got, c.want)
		}
		if got.Major() != c.major || got.Minor() != c.minor {
			t.Fatalf("%#x decoded to %d.%d, want %d.%d", got, got.Major(), got.Minor(), c.major, c.minor)
		}
	}
}

func TestWriteBlobNilVsEmpty(t *testing.T) {
	t.Run("nil is absent (-1)", func(t *testing.T) {
		var buf bytes.Buffer
		if err := writeBlob(newBinaryWriter(&buf), nil); err != nil {
			t.Fatal(err)
		}
		if n := int64(binary.LittleEndian.Uint64(buf.Bytes())); n != -1 {
			t.Fatalf("nil blob length = %d, want -1", n)
		}
	})
	t.Run("empty is present (0)", func(t *testing.T) {
		var buf bytes.Buffer
		if err := writeBlob(newBinaryWriter(&buf), []byte{}); err != nil {
			t.Fatal(err)
		}
		if n := int64(binary.LittleEndian.Uint64(buf.Bytes())); n != 0 {
			t.Fatalf("empty blob length = %d, want 0", n)
		}
	})
}

func headersEqual(a, b []Header) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
