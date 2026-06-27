package storage

import (
	"bytes"
	"hash/crc32"
)

var castagnoli = crc32.MakeTable(crc32.Castagnoli)

func recordChecksum(b []byte) uint32 {
	return crc32.Checksum(b, castagnoli)
}

// CaptureSessionWriter serializes a capture session to a .capture file. The file
// header is written when the writer is created; requests and responses are then
// appended as self-delimited records, each followed by a CRC32-C of its own
// bytes. See ARCHITECTURE.md ("Capture file format").
type CaptureSessionWriter interface {
	WriteRequest(r RequestRecord) error
	WriteResponse(r ResponseRecord) error
	Flush() error
	Close() error
}

type fileCaptureSessionWriter struct {
	file BinaryFileWriter
}

// NewFileCaptureSessionWriter opens (truncating) filepath and writes the capture
// header. httpsDecrypted records whether HTTPS bodies were MITM-decrypted.
func NewFileCaptureSessionWriter(filepath string, httpsDecrypted bool) (CaptureSessionWriter, error) {
	file, err := NewBinaryFileWriter(filepath)
	if err != nil {
		return nil, err
	}

	w := fileCaptureSessionWriter{file: file}
	if err := w.writeHeader(NewFileHeader(httpsDecrypted)); err != nil {
		_ = file.Close()
		return nil, err
	}
	return w, nil
}

func (w fileCaptureSessionWriter) writeHeader(h FileHeader) error {
	if _, err := w.file.Write(h.Magic[:]); err != nil {
		return err
	}
	if err := w.file.WriteInt16(h.Version); err != nil {
		return err
	}
	if err := w.file.WriteBool(h.HttpsDecrypted); err != nil {
		return err
	}
	return w.file.WriteInt32(h.RecordsCount)
}

func (w fileCaptureSessionWriter) WriteRequest(r RequestRecord) error {
	var buf bytes.Buffer
	b := newBinaryWriter(&buf)

	if err := b.WriteByte(byte(RecordTypeRequest)); err != nil {
		return err
	}
	if _, err := b.Write(r.RequestID[:]); err != nil {
		return err
	}
	if err := writeLPString(b, r.Method); err != nil {
		return err
	}
	if err := writeLPString(b, r.URL); err != nil {
		return err
	}
	if err := b.WriteByte(byte(r.HttpVersion)); err != nil {
		return err
	}
	if err := writeHeaders(b, r.Headers); err != nil {
		return err
	}
	if err := writeBlob(b, r.Body); err != nil {
		return err
	}

	return w.writeRecord(buf.Bytes())
}

func (w fileCaptureSessionWriter) WriteResponse(r ResponseRecord) error {
	var buf bytes.Buffer
	b := newBinaryWriter(&buf)

	if err := b.WriteByte(byte(RecordTypeResponse)); err != nil {
		return err
	}
	if _, err := b.Write(r.RequestID[:]); err != nil {
		return err
	}
	if err := b.WriteByte(byte(r.HttpVersion)); err != nil {
		return err
	}
	if err := b.WriteInt16(r.StatusCode); err != nil {
		return err
	}
	if err := writeLPString(b, r.StatusMessage); err != nil {
		return err
	}
	if err := writeHeaders(b, r.Headers); err != nil {
		return err
	}
	if err := writeBlob(b, r.Body); err != nil {
		return err
	}

	return w.writeRecord(buf.Bytes())
}

// writeRecord appends a framed record payload followed by its CRC32-C trailer.
func (w fileCaptureSessionWriter) writeRecord(payload []byte) error {
	if _, err := w.file.Write(payload); err != nil {
		return err
	}
	return w.file.WriteUint32(recordChecksum(payload))
}

func (w fileCaptureSessionWriter) Flush() error {
	return w.file.Flush()
}

func (w fileCaptureSessionWriter) Close() error {
	return w.file.Close()
}

// --- field encoders (see ARCHITECTURE.md "Conventions") ---

// writeLPString writes a uint32 byte length followed by the UTF-8 bytes.
func writeLPString(b binaryWriter, s string) error {
	if err := b.WriteUint32(uint32(len(s))); err != nil {
		return err
	}
	return b.WriteString(s)
}

// writeHeaders writes an int32 count followed by (name, value) lpstring pairs.
func writeHeaders(b binaryWriter, headers []Header) error {
	if err := b.WriteInt32(int32(len(headers))); err != nil {
		return err
	}
	for _, h := range headers {
		if err := writeLPString(b, h.Name); err != nil {
			return err
		}
		if err := writeLPString(b, h.Value); err != nil {
			return err
		}
	}
	return nil
}

// writeBlob writes an int64 byte length followed by the bytes. A nil slice is
// encoded as length -1 ("absent"); a non-nil empty slice as length 0.
func writeBlob(b binaryWriter, data []byte) error {
	if data == nil {
		return b.WriteInt64(-1)
	}
	if err := b.WriteInt64(int64(len(data))); err != nil {
		return err
	}
	_, err := b.Write(data)
	return err
}
