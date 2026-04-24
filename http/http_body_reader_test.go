package http

import (
	"bytes"
	"httpStackLens/http/models"
	"testing"
)

func TestReadHttpResponseBody(t *testing.T) {
	t.Run("Content-Length", func(t *testing.T) {
		body := "Hello World"
		head := models.HttpResponseHead{
			Headers: []models.Header{
				{Name: "Content-Length", Value: "11"},
			},
		}
		reader := bytes.NewReader([]byte(body))
		got, err := ReadHttpResponseBody(reader, head)
		if err != nil {
			t.Fatalf("ReadHttpResponseBody failed: %v", err)
		}
		if b, ok := got.(models.BodyString); !ok || b.Content != body {
			t.Errorf("got %v, want %s", got, body)
		}
	})

	t.Run("Chunked", func(t *testing.T) {
		body := "5\r\nHello\r\n6\r\n World\r\n0\r\n\r\n"
		expected := "Hello World"
		head := models.HttpResponseHead{
			Headers: []models.Header{
				{Name: "Transfer-Encoding", Value: "chunked"},
			},
		}
		reader := bytes.NewReader([]byte(body))
		got, err := ReadHttpResponseBody(reader, head)
		if err != nil {
			t.Fatalf("ReadHttpResponseBody failed: %v", err)
		}
		if b, ok := got.(models.BodyString); !ok || b.Content != expected {
			t.Errorf("got %v, want %s", got, expected)
		}
	})

	t.Run("Empty", func(t *testing.T) {
		head := models.HttpResponseHead{}
		reader := bytes.NewReader([]byte(""))
		got, err := ReadHttpResponseBody(reader, head)
		if err != nil {
			t.Fatalf("ReadHttpResponseBody failed: %v", err)
		}
		if _, ok := got.(models.EmptyBody); !ok {
			t.Errorf("got %T, want EmptyBody", got)
		}
	})
}
