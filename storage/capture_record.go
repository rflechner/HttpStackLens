package storage

import "crypto/rand"

// This file models the in-memory shape of everything that gets serialized into a
// .capture file. The on-disk binary layout is documented in ARCHITECTURE.md
// ("Capture file format"). Serialization itself lives in the writer/reader; these
// types are deliberately decoupled from the live proxy models so the file format
// can evolve independently.

// CaptureFileMagic is the 4-byte signature at the very start of a .capture file
// ("HSLC" = HttpStackLens Capture).
var CaptureFileMagic = [4]byte{'H', 'S', 'L', 'C'}

// CaptureFormatVersion is the current on-disk format version. Bump it whenever
// the layout changes; readers must reject versions they do not understand.
//
// v2 added the body_skipped flag to request and response records.
const CaptureFormatVersion int16 = 2

// FileHeader is the fixed-size header at the start of a .capture file.
type FileHeader struct {
	Magic          [4]byte // CaptureFileMagic
	Version        int16   // CaptureFormatVersion
	HttpsDecrypted bool    // whether HTTPS bodies were MITM-decrypted
	RecordsCount   int32   // number of records that follow (-1 = read until EOF)
}

// NewFileHeader builds a header for a fresh capture. RecordsCount is left at -1
// ("unknown / read until EOF") and can be backfilled when the file is closed.
func NewFileHeader(httpsDecrypted bool) FileHeader {
	return FileHeader{
		Magic:          CaptureFileMagic,
		Version:        CaptureFormatVersion,
		HttpsDecrypted: httpsDecrypted,
		RecordsCount:   -1,
	}
}

// RecordType is the 1-byte discriminator that prefixes every record and tells the
// reader whether a request or a response follows.
type RecordType uint8

const (
	RecordTypeRequest  RecordType = 0x01
	RecordTypeResponse RecordType = 0x02
)

// HttpVersion encodes the HTTP version in a single byte: the high nibble is the
// major and the low nibble the minor, so it maps directly onto major/minor
// integers (e.g. 0x11 = HTTP/1.1). 0x00 means unknown/unspecified.
type HttpVersion uint8

const (
	HttpVersionUnknown HttpVersion = 0x00
	HttpVersion10      HttpVersion = 0x10
	HttpVersion11      HttpVersion = 0x11
	HttpVersion20      HttpVersion = 0x20
	HttpVersion30      HttpVersion = 0x30
)

// NewHttpVersion packs a major/minor pair into the single-byte encoding. Values
// above 15 are truncated to their low nibble, which is fine for real HTTP.
func NewHttpVersion(major, minor int) HttpVersion {
	return HttpVersion(byte(major&0x0F)<<4 | byte(minor&0x0F))
}

// Major returns the major version number (high nibble).
func (v HttpVersion) Major() int { return int(v >> 4) }

// Minor returns the minor version number (low nibble).
func (v HttpVersion) Minor() int { return int(v & 0x0F) }

// UUID is the 16-byte identifier (RFC 4122 layout) that uniquely tags a request
// and links a response back to it. Stored raw, not as a formatted string.
type UUID [16]byte

// NewUUID returns a random (version 4) UUID.
func NewUUID() (UUID, error) {
	var u UUID
	if _, err := rand.Read(u[:]); err != nil {
		return u, err
	}
	u[6] = (u[6] & 0x0f) | 0x40 // version 4
	u[8] = (u[8] & 0x3f) | 0x80 // RFC 4122 variant
	return u, nil
}

// Header is a single HTTP header name/value pair. The slice order on a record
// preserves the on-the-wire order and any duplicates.
type Header struct {
	Name  string
	Value string
}

// CaptureRecord is implemented by the record types so a stream of mixed requests
// and responses can be handled uniformly. RecordType matches the on-disk
// discriminator.
type CaptureRecord interface {
	RecordType() RecordType
}

// RequestRecord is a captured HTTP request (record type 0x01).
type RequestRecord struct {
	RequestID   UUID        // unique id for this request
	Method      string      // "GET", "POST", "CONNECT", ...
	URL         string      // request target (absolute or origin form)
	HttpVersion HttpVersion // packed major/minor
	Headers     []Header    // request headers, in order
	// BodySkipped is true when the body was intentionally not stored (e.g. it
	// exceeded the configured size limit for its content type). Body is then nil.
	BodySkipped bool
	// Body is the request body. A nil slice means "absent" (encoded as length
	// -1); a non-nil empty slice means "present but empty" (length 0).
	Body []byte
}

func (RequestRecord) RecordType() RecordType { return RecordTypeRequest }

// ResponseRecord is a captured HTTP response (record type 0x02).
type ResponseRecord struct {
	RequestID     UUID        // links back to the RequestRecord it answers
	HttpVersion   HttpVersion // packed major/minor
	StatusCode    int16       // e.g. 200, 404
	StatusMessage string      // "OK", "Not Found", ...
	Headers       []Header    // response headers, in order
	// BodySkipped is true when the body was intentionally not stored (e.g. it
	// exceeded the configured size limit for its content type). Body is then nil.
	BodySkipped bool
	// Body is the response body. A nil slice means "absent" (encoded as length
	// -1); a non-nil empty slice means "present but empty" (length 0).
	Body []byte
}

func (ResponseRecord) RecordType() RecordType { return RecordTypeResponse }
