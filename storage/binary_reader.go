package storage

import (
	"encoding/binary"
	"io"
)

// binaryReader reads little-endian primitives from any io.Reader, mirroring
// binaryWriter. It is the low-level half of the capture reader.
type binaryReader struct {
	r io.Reader
}

func newBinaryReader(r io.Reader) binaryReader {
	return binaryReader{r: r}
}

// ReadByte reads a single byte. It returns io.EOF only when no byte was
// available (a clean boundary), and io.ErrUnexpectedEOF for a truncated read.
func (b binaryReader) ReadByte() (byte, error) {
	var buf [1]byte
	if _, err := io.ReadFull(b.r, buf[:]); err != nil {
		return 0, err
	}
	return buf[0], nil
}

func (b binaryReader) ReadBool() (bool, error) {
	v, err := b.ReadByte()
	return v != 0, err
}

func (b binaryReader) ReadInt16() (int16, error) {
	var v int16
	err := binary.Read(b.r, binary.LittleEndian, &v)
	return v, err
}

func (b binaryReader) ReadInt32() (int32, error) {
	var v int32
	err := binary.Read(b.r, binary.LittleEndian, &v)
	return v, err
}

func (b binaryReader) ReadInt64() (int64, error) {
	var v int64
	err := binary.Read(b.r, binary.LittleEndian, &v)
	return v, err
}

func (b binaryReader) ReadUint32() (uint32, error) {
	var v uint32
	err := binary.Read(b.r, binary.LittleEndian, &v)
	return v, err
}

// ReadBytes reads exactly n bytes.
func (b binaryReader) ReadBytes(n int) ([]byte, error) {
	if n == 0 {
		return nil, nil
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(b.r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}
