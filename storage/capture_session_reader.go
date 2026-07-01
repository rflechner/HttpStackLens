package storage

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
)

// maxFieldLen caps a single length-prefixed field (string or blob), so a corrupt
// length cannot make the reader allocate unbounded memory before the CRC check
// gets a chance to run.
const maxFieldLen = 256 << 20 // 256 MiB

var (
	// ErrBadMagic is returned when the file does not start with CaptureFileMagic.
	ErrBadMagic = errors.New("storage: not a capture file (bad magic)")
	// ErrUnsupportedVersion is returned for a format version the reader does not
	// understand. The offending version is wrapped in the error message.
	ErrUnsupportedVersion = errors.New("storage: unsupported capture version")
)

// CorruptRecordError signals that a record's CRC32-C trailer did not match its
// contents. A caller may choose to stop, or to skip and keep reading.
type CorruptRecordError struct {
	Stored   uint32
	Computed uint32
}

func (e *CorruptRecordError) Error() string {
	return fmt.Sprintf("storage: corrupt record (crc stored %#x, computed %#x)", e.Stored, e.Computed)
}

// CaptureSessionReader reads a .capture file produced by CaptureSessionWriter.
// The header is parsed on open; Read returns records one at a time and io.EOF
// once the stream is exhausted.
type CaptureSessionReader interface {
	Header() FileHeader
	Read() (CaptureRecord, error)
	Close() error
}

type fileCaptureSessionReader struct {
	file   *os.File
	r      *bufio.Reader
	header FileHeader
}

// NewFileCaptureSessionReader opens filepath, reads and validates the header, and
// returns a reader positioned at the first record.
func NewFileCaptureSessionReader(filepath string) (CaptureSessionReader, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}

	r := bufio.NewReader(file)
	header, err := readHeader(newBinaryReader(r))
	if err != nil {
		_ = file.Close()
		return nil, err
	}

	return &fileCaptureSessionReader{file: file, r: r, header: header}, nil
}

func (cr *fileCaptureSessionReader) Header() FileHeader { return cr.header }

func (cr *fileCaptureSessionReader) Close() error { return cr.file.Close() }

// Read parses the next record and verifies its checksum. It returns io.EOF at a
// clean record boundary, io.ErrUnexpectedEOF on truncation, and a
// *CorruptRecordError on a checksum mismatch.
func (cr *fileCaptureSessionReader) Read() (CaptureRecord, error) {
	// Tee the record payload into a buffer as we parse it, so we can checksum it
	// against the trailer that follows.
	var payload bytes.Buffer
	br := newBinaryReader(io.TeeReader(cr.r, &payload))

	recordType, err := br.ReadByte()
	if err != nil {
		return nil, err // io.EOF here means "no more records"
	}

	var record CaptureRecord
	switch RecordType(recordType) {
	case RecordTypeRequest:
		record, err = readRequest(br)
	case RecordTypeResponse:
		record, err = readResponse(br)
	default:
		return nil, fmt.Errorf("storage: unknown record type %#x", recordType)
	}
	if err != nil {
		return nil, unexpectedEOF(err)
	}

	// The trailing CRC is not part of the checksummed payload, so read it from
	// the underlying reader rather than through the tee.
	storedCRC, err := newBinaryReader(cr.r).ReadUint32()
	if err != nil {
		return nil, unexpectedEOF(err)
	}
	if computed := recordChecksum(payload.Bytes()); computed != storedCRC {
		return nil, &CorruptRecordError{Stored: storedCRC, Computed: computed}
	}

	return record, nil
}

func readHeader(br binaryReader) (FileHeader, error) {
	var h FileHeader

	magic, err := br.ReadBytes(4)
	if err != nil {
		return h, err
	}
	copy(h.Magic[:], magic)
	if h.Magic != CaptureFileMagic {
		return h, ErrBadMagic
	}
	if h.Version, err = br.ReadInt16(); err != nil {
		return h, err
	}
	if h.Version != CaptureFormatVersion {
		return h, fmt.Errorf("%w: %d", ErrUnsupportedVersion, h.Version)
	}
	if h.HttpsDecrypted, err = br.ReadBool(); err != nil {
		return h, err
	}
	if h.RecordsCount, err = br.ReadInt32(); err != nil {
		return h, err
	}
	return h, nil
}

func readRequest(br binaryReader) (RequestRecord, error) {
	var r RequestRecord

	id, err := br.ReadBytes(16)
	if err != nil {
		return r, err
	}
	copy(r.RequestID[:], id)

	if r.Method, err = readLPString(br); err != nil {
		return r, err
	}
	if r.URL, err = readLPString(br); err != nil {
		return r, err
	}
	v, err := br.ReadByte()
	if err != nil {
		return r, err
	}
	r.HttpVersion = HttpVersion(v)
	if r.Headers, err = readHeaders(br); err != nil {
		return r, err
	}
	if r.BodySkipped, err = br.ReadBool(); err != nil {
		return r, err
	}
	if r.Body, err = readBlob(br); err != nil {
		return r, err
	}
	return r, nil
}

func readResponse(br binaryReader) (ResponseRecord, error) {
	var r ResponseRecord

	id, err := br.ReadBytes(16)
	if err != nil {
		return r, err
	}
	copy(r.RequestID[:], id)

	v, err := br.ReadByte()
	if err != nil {
		return r, err
	}
	r.HttpVersion = HttpVersion(v)
	if r.StatusCode, err = br.ReadInt16(); err != nil {
		return r, err
	}
	if r.StatusMessage, err = readLPString(br); err != nil {
		return r, err
	}
	if r.Headers, err = readHeaders(br); err != nil {
		return r, err
	}
	if r.BodySkipped, err = br.ReadBool(); err != nil {
		return r, err
	}
	if r.Body, err = readBlob(br); err != nil {
		return r, err
	}
	return r, nil
}

// --- field decoders (mirror the encoders in capture_session_writer.go) ---

func readLPString(br binaryReader) (string, error) {
	n, err := br.ReadUint32()
	if err != nil {
		return "", err
	}
	if n > maxFieldLen {
		return "", fmt.Errorf("storage: string length %d exceeds limit", n)
	}
	bytes, err := br.ReadBytes(int(n))
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func readHeaders(br binaryReader) ([]Header, error) {
	n, err := br.ReadInt32()
	if err != nil {
		return nil, err
	}
	if n < 0 {
		return nil, fmt.Errorf("storage: negative header count %d", n)
	}
	if n == 0 {
		return nil, nil
	}
	headers := make([]Header, 0, n)
	for i := int32(0); i < n; i++ {
		name, err := readLPString(br)
		if err != nil {
			return nil, err
		}
		value, err := readLPString(br)
		if err != nil {
			return nil, err
		}
		headers = append(headers, Header{Name: name, Value: value})
	}
	return headers, nil
}

func readBlob(br binaryReader) ([]byte, error) {
	n, err := br.ReadInt64()
	if err != nil {
		return nil, err
	}
	if n < 0 {
		return nil, nil // absent
	}
	if n > maxFieldLen {
		return nil, fmt.Errorf("storage: blob length %d exceeds limit", n)
	}
	return br.ReadBytes(int(n))
}

// unexpectedEOF maps a mid-record io.EOF to io.ErrUnexpectedEOF, since hitting
// EOF while parsing a record means the file is truncated, not finished.
func unexpectedEOF(err error) error {
	if errors.Is(err, io.EOF) {
		return io.ErrUnexpectedEOF
	}
	return err
}
