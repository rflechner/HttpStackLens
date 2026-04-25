package http

import (
	"bufio"
	"io"
	"net"
)

type NetworkBuffer struct {
	bytes  []byte
	length int
}

type NetworkStream struct {
	reader *bufio.Reader
	writer *bufio.Writer
}

func NewNetworkStream(conn net.Conn) *NetworkStream {
	return &NetworkStream{
		reader: bufio.NewReader(conn),
		writer: bufio.NewWriter(conn),
	}
}

func (s *NetworkStream) ReadBytesCount(buffer *[]byte, maxBufferLength int) (NetworkBuffer, error) {
	if cap(*buffer) < maxBufferLength {
		*buffer = make([]byte, maxBufferLength)
	} else {
		*buffer = (*buffer)[:maxBufferLength]
	}

	n, err := io.ReadFull(s.reader, *buffer)
	return NetworkBuffer{
		bytes:  *buffer,
		length: n,
	}, err
}

func (s *NetworkStream) ReadLine() (string, error) {
	line, isPrefix, err := s.reader.ReadLine()
	if err != nil {
		return "", err
	}
	if !isPrefix {
		return string(line), nil
	}

	// Line is too long for bufio.Reader's buffer, read the rest
	var res []byte
	res = append(res, line...)
	for isPrefix {
		line, isPrefix, err = s.reader.ReadLine()
		if err != nil {
			return string(res), err
		}
		res = append(res, line...)
	}
	return string(res), nil
}

func (s *NetworkStream) Read(p []byte) (n int, err error) {
	return s.reader.Read(p)
}

func (s *NetworkStream) Write(p []byte) (n int, err error) {
	n, err = s.writer.Write(p)
	if err != nil {
		return n, err
	}
	err = s.writer.Flush()
	return n, err
}

func (s *NetworkStream) Copy(dst io.Writer) (int64, error) {
	return s.reader.WriteTo(dst)
}
