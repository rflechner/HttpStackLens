package models

import (
	"bytes"
	"strings"
	"testing"
)

func TestProxyRequestWriteToRequestTargetForms(t *testing.T) {
	cases := []struct {
		name     string
		method   HttpMethod
		endpoint ResourceEndpoint
		toProxy  bool
		want     string
	}{
		{"HTTP default", GET, ResourceEndpoint{Host: "example.com", Port: 80}, true, "GET http://example.com/ HTTP/1.1"},
		{"direct empty path", GET, ResourceEndpoint{Host: "example.com", Port: 80}, false, "GET / HTTP/1.1"},
		{"proxy query without path", GET, ResourceEndpoint{Host: "example.com", Port: 80, PathAndQuery: "?format=json"}, true, "GET http://example.com/?format=json HTTP/1.1"},
		{"direct query without path", GET, ResourceEndpoint{Host: "example.com", Port: 80, PathAndQuery: "?format=json"}, false, "GET /?format=json HTTP/1.1"},
		{"proxy OPTIONS", OPTIONS, ResourceEndpoint{Host: "example.com", Port: 80, PathAndQuery: "*"}, true, "OPTIONS * HTTP/1.1"},
		{"direct OPTIONS", OPTIONS, ResourceEndpoint{Host: "example.com", Port: 80, PathAndQuery: "*"}, false, "OPTIONS * HTTP/1.1"},
		{"IPv6 CONNECT", CONNECT, ResourceEndpoint{Host: "2001:db8::1", Port: 443}, true, "CONNECT [2001:db8::1]:443 HTTP/1.1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := ProxyRequest{HttpRequestLine: HttpRequestLine{
				HttpMethod: tc.method, Endpoint: tc.endpoint, Version: Version{Major: 1, Minor: 1},
			}}
			var buf bytes.Buffer
			n, err := request.WriteTo(&buf, tc.toProxy)
			if err != nil {
				t.Fatal(err)
			}
			if n != buf.Len() || !strings.HasPrefix(buf.String(), tc.want+"\r\n") {
				t.Fatalf("WriteTo = (%d, %q), want prefix %q", n, buf.String(), tc.want)
			}
		})
	}
}

func TestProxyRequest_WriteTo(t *testing.T) {
	req := ProxyRequest{
		HttpRequestLine: HttpRequestLine{
			HttpMethod: CONNECT,
			Endpoint:   ResourceEndpoint{Host: "example.com", Port: 443},
			Version:    Version{Major: 1, Minor: 1},
		},
		Headers: []Header{
			{Name: "Host", Value: "example.com:443"},
			{Name: "Proxy-Authorization", Value: "Basic qwertztrfedsaf"},
			{Name: "Proxy-Connection", Value: "Keep-Alive"},
			{Name: "User-Agent", Value: "curl/7.68.0"},
		},
	}

	t.Run("with writeProxyHeader=true", func(t *testing.T) {
		var buf bytes.Buffer
		_, err := req.WriteTo(&buf, true)
		if err != nil {
			t.Fatalf("WriteTo failed: %v", err)
		}

		expected := "CONNECT example.com:443 HTTP/1.1\r\n" +
			"Host: example.com:443\r\n" +
			"Proxy-Authorization: Basic qwertztrfedsaf\r\n" +
			"Proxy-Connection: Keep-Alive\r\n" +
			"User-Agent: curl/7.68.0\r\n" +
			"\r\n"

		if buf.String() != expected {
			t.Errorf("Unexpected output:\nGot:  %q\nWant: %q", buf.String(), expected)
		}
	})

	t.Run("with writeProxyHeader=false", func(t *testing.T) {
		var buf bytes.Buffer
		_, err := req.WriteTo(&buf, false)
		if err != nil {
			t.Fatalf("WriteTo failed: %v", err)
		}

		// Proxy- headers should be filtered
		expected := "CONNECT example.com:443 HTTP/1.1\r\n" +
			"Host: example.com:443\r\n" +
			"User-Agent: curl/7.68.0\r\n" +
			"\r\n"

		if buf.String() != expected {
			t.Errorf("Unexpected output:\nGot:  %q\nWant: %q", buf.String(), expected)
		}
	})
}

func TestProxyRequest_SetHeaderReplacesExistingHeaderCaseInsensitively(t *testing.T) {
	request := ProxyRequest{
		Headers: []Header{
			{Name: "Proxy-Authorization", Value: "Negotiate old-token"},
		},
	}

	request.SetHeader("proxy-authorization", "Negotiate new-token")

	headers := request.GetHeader("Proxy-Authorization")
	if len(headers) != 1 {
		t.Fatalf("expected one Proxy-Authorization header, got %d", len(headers))
	}
	if headers[0] != "Negotiate new-token" {
		t.Fatalf("expected header value to be replaced, got %s", headers[0])
	}
}

func TestProxyRequest_SetHeaderAddsMissingHeader(t *testing.T) {
	request := ProxyRequest{}

	request.SetHeader("Authorization", "Negotiate token")

	headers := request.GetHeader("Authorization")
	if len(headers) != 1 {
		t.Fatalf("expected one Authorization header, got %d", len(headers))
	}
	if headers[0] != "Negotiate token" {
		t.Fatalf("expected header value to be added, got %s", headers[0])
	}
}
