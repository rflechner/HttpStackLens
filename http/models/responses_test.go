package models

import (
	"strings"
	"testing"
)

func TestResponseHead_String(t *testing.T) {
	tests := []struct {
		name string
		h    *HttpResponseHead
		want string
	}{
		{
			name: "Simple OK without headers",
			h: &HttpResponseHead{
				HttpVersion:       Version{Major: 1, Minor: 1},
				StatusCode:        200,
				StatusDescription: "OK",
			},
			want: "HTTP/1.1 200 OK\r\n\r\n",
		},
		{
			name: "OK with multiple headers",
			h: &HttpResponseHead{
				HttpVersion:       Version{Major: 1, Minor: 1},
				StatusCode:        200,
				StatusDescription: "OK",
				Headers: []Header{
					{Name: "Content-Type", Value: "application/json"},
					{Name: "Content-Length", Value: "42"},
				},
			},
			want: "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 42\r\n\r\n",
		},
		{
			name: "HTTP/2.0 Redirect with header",
			h: &HttpResponseHead{
				HttpVersion:       Version{Major: 2, Minor: 0},
				StatusCode:        301,
				StatusDescription: "Moved Permanently",
				Headers: []Header{
					{Name: "Location", Value: "https://example.com"},
				},
			},
			want: "HTTP/2.0 301 Moved Permanently\r\nLocation: https://example.com\r\n\r\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.h.String(); got != tt.want {
				t.Errorf("HttpResponseHead.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResponseHead_GetHeader(t *testing.T) {
	h := &HttpResponseHead{
		Headers: []Header{
			{Name: "Content-Type", Value: "application/json"},
			{Name: "Content-Length", Value: "42"},
			{Name: "X-Forwarded-For", Value: "127.0.0.1"},
			{Name: "X-Forwarded-For", Value: "10.0.0.1"},
		},
	}

	tests := []struct {
		name string
		key  string
		want []string
	}{
		{
			name: "Existing header (case sensitive)",
			key:  "Content-Type",
			want: []string{"application/json"},
		},
		{
			name: "Existing header (another)",
			key:  "Content-Length",
			want: []string{"42"},
		},
		{
			name: "Non-existing header",
			key:  "Authorization",
			want: nil,
		},
		{
			name: "Existing header (different case)",
			key:  "content-type",
			want: []string{"application/json"},
		},
		{
			name: "Multiple values for header",
			key:  "X-Forwarded-For",
			want: []string{"127.0.0.1", "10.0.0.1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := h.GetHeader(tt.key); !slicesEqual(got, tt.want) {
				t.Errorf("HttpResponseHead.GetHeader(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestResponseHead_SetContentLength(t *testing.T) {
	t.Run("Add Content-Length if not exists", func(t *testing.T) {
		h := &HttpResponseHead{}
		h.SetContentLength(1024)
		got := h.GetHeader("Content-Length")
		want := []string{"1024"}
		if !slicesEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("Replace existing Content-Length", func(t *testing.T) {
		h := &HttpResponseHead{
			Headers: []Header{
				{Name: "Content-Length", Value: "0"},
			},
		}
		h.SetContentLength(2048)
		got := h.GetHeader("Content-Length")
		want := []string{"2048"}
		if !slicesEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("Replace existing Content-Length (case insensitive)", func(t *testing.T) {
		h := &HttpResponseHead{
			Headers: []Header{
				{Name: "content-length", Value: "0"},
			},
		}
		h.SetContentLength(4096)
		got := h.GetHeader("Content-Length")
		want := []string{"4096"}
		if !slicesEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})
}

func TestHttpResponse_WriteTo(t *testing.T) {
	t.Run("Write response with BodyString", func(t *testing.T) {
		r := HttpResponse{
			Head: HttpResponseHead{
				HttpVersion:       Version{Major: 1, Minor: 1},
				StatusCode:        200,
				StatusDescription: "OK",
				Headers: []Header{
					{Name: "Content-Type", Value: "text/plain"},
				},
			},
			Body: BodyString{Content: "Hello, World!"},
		}

		var buf strings.Builder
		n, err := r.WriteTo(&buf)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got := buf.String()
		want := "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 13\r\n\r\nHello, World!"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}

		if n != int64(len(want)) {
			t.Errorf("got length %d, want %d", n, len(want))
		}
	})

	t.Run("Write response with EmptyBody", func(t *testing.T) {
		r := HttpResponse{
			Head: HttpResponseHead{
				HttpVersion:       Version{Major: 1, Minor: 1},
				StatusCode:        204,
				StatusDescription: "No Content",
			},
			Body: EmptyBody{},
		}

		var buf strings.Builder
		n, err := r.WriteTo(&buf)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got := buf.String()
		want := "HTTP/1.1 204 No Content\r\n\r\n"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}

		if n != int64(len(want)) {
			t.Errorf("got length %d, want %d", n, len(want))
		}
	})
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
