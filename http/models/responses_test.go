package models

import (
	"testing"
)

func TestResponseHead_String(t *testing.T) {
	tests := []struct {
		name string
		h    *ResponseHead
		want string
	}{
		{
			name: "Simple OK without headers",
			h: &ResponseHead{
				HttpVersion:       Version{Major: 1, Minor: 1},
				StatusCode:        200,
				StatusDescription: "OK",
			},
			want: "HTTP/1.1 200 OK\r\n\r\n",
		},
		{
			name: "OK with multiple headers",
			h: &ResponseHead{
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
			h: &ResponseHead{
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
				t.Errorf("ResponseHead.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResponseHead_GetHeader(t *testing.T) {
	h := &ResponseHead{
		Headers: []Header{
			{Name: "Content-Type", Value: "application/json"},
			{Name: "Content-Length", Value: "42"},
		},
	}

	tests := []struct {
		name string
		key  string
		want string
	}{
		{
			name: "Existing header (case sensitive)",
			key:  "Content-Type",
			want: "application/json",
		},
		{
			name: "Existing header (another)",
			key:  "Content-Length",
			want: "42",
		},
		{
			name: "Non-existing header",
			key:  "Authorization",
			want: "",
		},
		{
			name: "Existing header (different case)",
			key:  "content-type",
			want: "application/json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := h.GetHeader(tt.key); got != tt.want {
				t.Errorf("ResponseHead.GetHeader(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}
