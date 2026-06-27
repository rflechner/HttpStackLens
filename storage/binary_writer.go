package storage

import (
	"encoding/binary"
	"io"
	"os"
)

// binaryWriter writes little-endian primitives to any io.Writer. It backs both
// the file writer and the in-memory buffers used to frame individual records
// before their checksum is computed.
type binaryWriter struct {
	w io.Writer
}

func newBinaryWriter(w io.Writer) binaryWriter {
	return binaryWriter{w: w}
}

func (b binaryWriter) WriteBool(v bool) error {
	if v {
		_, err := b.w.Write([]byte{1})
		return err
	}
	_, err := b.w.Write([]byte{0})
	return err
}

func (b binaryWriter) WriteByte(v byte) error {
	_, err := b.w.Write([]byte{v})
	return err
}

func (b binaryWriter) WriteInt8(v int8) error {
	return binary.Write(b.w, binary.LittleEndian, v)
}

func (b binaryWriter) WriteInt16(v int16) error {
	return binary.Write(b.w, binary.LittleEndian, v)
}

func (b binaryWriter) WriteInt32(v int32) error {
	return binary.Write(b.w, binary.LittleEndian, v)
}

func (b binaryWriter) WriteInt64(v int64) error {
	return binary.Write(b.w, binary.LittleEndian, v)
}

func (b binaryWriter) WriteUint32(v uint32) error {
	return binary.Write(b.w, binary.LittleEndian, v)
}

func (b binaryWriter) WriteString(v string) error {
	_, err := io.WriteString(b.w, v)
	return err
}

func (b binaryWriter) Write(p []byte) (int, error) {
	return b.w.Write(p)
}

// binaryFileWriter is a binaryWriter targeting a file, with flush/close on top.
type binaryFileWriter struct {
	binaryWriter
	file *os.File
}

type BinaryFileWriter interface {
	WriteBool(v bool) error
	WriteByte(v byte) error
	WriteInt8(v int8) error
	WriteInt16(v int16) error
	WriteInt32(v int32) error
	WriteInt64(v int64) error
	WriteUint32(v uint32) error
	WriteString(v string) error
	Write(b []byte) (int, error)
	Flush() error
	Close() error
}

func NewBinaryFileWriter(filepath string) (BinaryFileWriter, error) {
	file, err := os.OpenFile(filepath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	return binaryFileWriter{
		binaryWriter: newBinaryWriter(file),
		file:         file,
	}, nil
}

func (w binaryFileWriter) Flush() error {
	return w.file.Sync()
}

func (w binaryFileWriter) Close() error {
	return w.file.Close()
}
