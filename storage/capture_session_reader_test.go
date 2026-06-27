package storage

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCaptureSessionReaderRoundTrip(t *testing.T) {
	path, req, resp := writeSampleCapture(t, true)

	reader, err := NewFileCaptureSessionReader(path)
	if err != nil {
		t.Fatalf("NewFileCaptureSessionReader: %v", err)
	}
	defer reader.Close()

	// Header.
	h := reader.Header()
	if h.Magic != CaptureFileMagic {
		t.Fatalf("magic = %v, want %v", h.Magic, CaptureFileMagic)
	}
	if h.Version != CaptureFormatVersion {
		t.Fatalf("version = %d, want %d", h.Version, CaptureFormatVersion)
	}
	if !h.HttpsDecrypted {
		t.Fatalf("https_decrypted = false, want true")
	}

	// First record: request.
	rec, err := reader.Read()
	if err != nil {
		t.Fatalf("Read request: %v", err)
	}
	gotReq, ok := rec.(RequestRecord)
	if !ok {
		t.Fatalf("first record is %T, want RequestRecord", rec)
	}
	if !reflect.DeepEqual(gotReq, req) {
		t.Fatalf("request mismatch:\n got  %+v\n want %+v", gotReq, req)
	}

	// Second record: response.
	rec, err = reader.Read()
	if err != nil {
		t.Fatalf("Read response: %v", err)
	}
	gotResp, ok := rec.(ResponseRecord)
	if !ok {
		t.Fatalf("second record is %T, want ResponseRecord", rec)
	}
	if !reflect.DeepEqual(gotResp, resp) {
		t.Fatalf("response mismatch:\n got  %+v\n want %+v", gotResp, resp)
	}

	// End of stream.
	if _, err := reader.Read(); !errors.Is(err, io.EOF) {
		t.Fatalf("end of stream: got %v, want io.EOF", err)
	}
}

func TestCaptureSessionReaderRejectsBadMagic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.capture")
	if err := os.WriteFile(path, []byte("NOPExxxxxxx"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := NewFileCaptureSessionReader(path)
	if !errors.Is(err, ErrBadMagic) {
		t.Fatalf("got %v, want ErrBadMagic", err)
	}
}

func TestCaptureSessionReaderRejectsUnsupportedVersion(t *testing.T) {
	path, _, _ := writeSampleCapture(t, false)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Bump the version (int16 at offset 4) to an unknown value.
	data[4] = 0x7F
	data[5] = 0x00
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = NewFileCaptureSessionReader(path)
	if !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatalf("got %v, want ErrUnsupportedVersion", err)
	}
}

func TestCaptureSessionReaderDetectsCorruption(t *testing.T) {
	path, _, _ := writeSampleCapture(t, false)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Flip a byte in the first record's payload (header is 11 bytes, +1 for the
	// record-type byte → corrupt the request id).
	data[12] ^= 0xFF
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	reader, err := NewFileCaptureSessionReader(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer reader.Close()

	_, err = reader.Read()
	var corrupt *CorruptRecordError
	if !errors.As(err, &corrupt) {
		t.Fatalf("got %v, want *CorruptRecordError", err)
	}
}

func TestCaptureSessionReaderTruncated(t *testing.T) {
	path, _, _ := writeSampleCapture(t, false)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Cut the file mid-way through the first record.
	if err := os.WriteFile(path, data[:15], 0o644); err != nil {
		t.Fatal(err)
	}

	reader, err := NewFileCaptureSessionReader(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer reader.Close()

	if _, err := reader.Read(); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("got %v, want io.ErrUnexpectedEOF", err)
	}
}
