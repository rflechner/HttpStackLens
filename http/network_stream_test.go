package http

import (
	"bytes"
	"net"
	"testing"
)

func createMockConn(data string) net.Conn {
	client, server := net.Pipe()
	go func() {
		server.Write([]byte(data))
		server.Close()
	}()
	return client
}

func TestNetworkStreamReader_ReadLine(t *testing.T) {
	data := "Line 1\r\nLine 2\nLine 3"
	conn := createMockConn(data)
	defer conn.Close()
	reader := NewNetworkStream(conn)

	line, err := reader.ReadLine()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if line != "Line 1" {
		t.Errorf("Expected 'Line 1', got '%s'", line)
	}

	line, err = reader.ReadLine()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if line != "Line 2" {
		t.Errorf("Expected 'Line 2', got '%s'", line)
	}

	line, err = reader.ReadLine()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if line != "Line 3" {
		t.Errorf("Expected 'Line 3', got '%s'", line)
	}
}

func TestNetworkStreamReader_ReadBytesCount(t *testing.T) {
	data := "Hello World"
	conn := createMockConn(data)
	defer conn.Close()
	reader := NewNetworkStream(conn)

	buffer := make([]byte, 0)
	nb, err := reader.ReadBytesCount(&buffer, 5)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if string(nb.bytes[:nb.length]) != "Hello" {
		t.Errorf("Expected 'Hello', got '%s'", string(nb.bytes[:nb.length]))
	}
	if nb.length != 5 {
		t.Errorf("Expected length 5, got %d", nb.length)
	}

	nb, err = reader.ReadBytesCount(&buffer, 6)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if string(nb.bytes[:nb.length]) != " World" {
		t.Errorf("Expected ' World', got '%s'", string(nb.bytes[:nb.length]))
	}
}

func TestNetworkStreamReader_Read(t *testing.T) {
	data := "Testing Read"
	conn := createMockConn(data)
	defer conn.Close()
	reader := NewNetworkStream(conn)

	p := make([]byte, 7)
	n, err := reader.Read(p)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if n != 7 {
		t.Errorf("Expected 7 bytes, got %d", n)
	}
	if string(p) != "Testing" {
		t.Errorf("Expected 'Testing', got '%s'", string(p))
	}
}

func TestNetworkStreamReader_MixedRead(t *testing.T) {
	data := "Header: value\r\nBodyContent"
	conn := createMockConn(data)
	defer conn.Close()
	reader := NewNetworkStream(conn)

	// 1. Lire la ligne
	line, err := reader.ReadLine()
	if err != nil {
		t.Fatalf("Expected no error reading line, got %v", err)
	}
	if line != "Header: value" {
		t.Errorf("Expected 'Header: value', got '%s'", line)
	}

	// 2. Lire les bytes restants (le corps)
	buffer := make([]byte, 0)
	nb, err := reader.ReadBytesCount(&buffer, 11)
	if err != nil {
		t.Fatalf("Expected no error reading bytes, got %v", err)
	}
	if string(nb.bytes[:nb.length]) != "BodyContent" {
		t.Errorf("Expected 'BodyContent', got '%s'", string(nb.bytes[:nb.length]))
	}
	if nb.length != 11 {
		t.Errorf("Expected length 11, got %d", nb.length)
	}
}

func TestNetworkStream_Copy(t *testing.T) {
	data := "Header: value\r\nRest of the data"
	conn := createMockConn(data)
	defer conn.Close()
	stream := NewNetworkStream(conn)

	// 1. Lire une partie pour remplir le buffer interne
	line, err := stream.ReadLine()
	if err != nil {
		t.Fatalf("Expected no error reading line, got %v", err)
	}
	if line != "Header: value" {
		t.Errorf("Expected 'Header: value', got '%s'", line)
	}

	// 2. Copier le reste
	var dst bytes.Buffer
	n, err := stream.Copy(&dst)
	if err != nil {
		t.Fatalf("Expected no error copying, got %v", err)
	}

	expectedRest := "Rest of the data"
	if dst.String() != expectedRest {
		t.Errorf("Expected '%s', got '%s'", expectedRest, dst.String())
	}
	if n != int64(len(expectedRest)) {
		t.Errorf("Expected %d bytes copied, got %d", len(expectedRest), n)
	}
}
