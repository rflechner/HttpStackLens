package http

import (
	"bufio"
	"io"
	"net"
	"time"
)

type NetworkBuffer struct {
	bytes  []byte
	length int
}

// NetworkStream wraps a net.Conn with a single, persistent buffered reader and
// writer. It implements net.Conn itself so it can be threaded through code that
// expects a connection while guaranteeing every read goes through the same
// buffer. This avoids losing bytes that the buffered reader has already pulled
// off the socket (e.g. a request body read together with its headers).
type NetworkStream struct {
	conn   net.Conn
	reader *bufio.Reader
	writer *bufio.Writer
}

func NewNetworkStream(conn net.Conn) *NetworkStream {
	return &NetworkStream{
		conn:   conn,
		reader: bufio.NewReader(conn),
		writer: bufio.NewWriter(conn),
	}
}

// AsNetworkStream returns conn unchanged when it is already a *NetworkStream,
// otherwise it wraps it in a new one. This prevents layering a second buffer
// over an already-buffered connection, which would discard bytes the existing
// buffer has already consumed.
func AsNetworkStream(conn net.Conn) *NetworkStream {
	if stream, ok := conn.(*NetworkStream); ok {
		return stream
	}
	return NewNetworkStream(conn)
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

// net.Conn interface: Read and Write are defined above; the methods below
// delegate to the underlying connection so a NetworkStream can be used anywhere
// a net.Conn is expected.

func (s *NetworkStream) Close() error {
	return s.conn.Close()
}

func (s *NetworkStream) LocalAddr() net.Addr {
	return s.conn.LocalAddr()
}

func (s *NetworkStream) RemoteAddr() net.Addr {
	return s.conn.RemoteAddr()
}

func (s *NetworkStream) SetDeadline(t time.Time) error {
	return s.conn.SetDeadline(t)
}

func (s *NetworkStream) SetReadDeadline(t time.Time) error {
	return s.conn.SetReadDeadline(t)
}

func (s *NetworkStream) SetWriteDeadline(t time.Time) error {
	return s.conn.SetWriteDeadline(t)
}
