package storage

import (
	"hash/crc32"
)

var castagnoli = crc32.MakeTable(crc32.Castagnoli)

func recordChecksum(b []byte) uint32 {
	return crc32.Checksum(b, castagnoli)
}

type fileCaptureSessionWriter struct {
	file BinaryFileWriter
}

type CaptureSessionWriter interface {
	Flush() error
	Close() error
}

func NewFileCaptureSessionWriter(filepath string) (CaptureSessionWriter, error) {
	file, err := NewBinaryFileWriter(filepath)
	if err != nil {
		return nil, err
	}

	return fileCaptureSessionWriter{
		file: file,
	}, nil
}

func (w fileCaptureSessionWriter) Flush() error {
	return w.file.Flush()
}

func (w fileCaptureSessionWriter) Close() error {
	return w.file.Close()
}
