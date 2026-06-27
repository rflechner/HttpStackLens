package models

import (
	"bytes"
	"testing"
)

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
