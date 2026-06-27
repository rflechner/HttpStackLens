package storage

import (
	"encoding/binary"
	"os"
)

type binaryFileWriter struct {
	file *os.File
}

type BinaryFileWriter interface {
	WriteBool(v bool) error
	WriteInt8(v int8) error
	WriteInt16(v int16) error
	WriteInt32(v int32) error
	WriteInt64(v int64) error
	WriteString(v string) error
	Write(b []byte) (int, error)
	Flush() error
	Close() error
}

func NewBinaryFileWriter(filepath string) (BinaryFileWriter, error) {
	file, err := os.OpenFile(filepath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	return binaryFileWriter{
		file: file,
	}, nil
}

func (w binaryFileWriter) WriteBool(v bool) error {
	if v {
		_, err := w.file.Write([]byte{1})
		return err
	}
	_, err := w.file.Write([]byte{0})
	return err
}

func (w binaryFileWriter) WriteInt8(v int8) error {
	return binary.Write(w.file, binary.LittleEndian, v)
}

func (w binaryFileWriter) WriteInt16(v int16) error {
	return binary.Write(w.file, binary.LittleEndian, v)
}

func (w binaryFileWriter) WriteInt32(v int32) error {
	return binary.Write(w.file, binary.LittleEndian, v)
}

func (w binaryFileWriter) WriteInt64(v int64) error {
	return binary.Write(w.file, binary.LittleEndian, v)
}

func (w binaryFileWriter) WriteString(v string) error {
	_, err := w.file.WriteString(v)
	return err
}

func (w binaryFileWriter) Write(b []byte) (int, error) {
	return w.file.Write(b)
}

func (w binaryFileWriter) Flush() error {
	return w.file.Sync()
}

func (w binaryFileWriter) Close() error {
	return w.file.Close()
}
