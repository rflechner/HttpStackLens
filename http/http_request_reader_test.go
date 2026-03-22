package http

import (
	"httpStackLens/http/ast"
	"strings"
	"testing"
)

func TestReadProxyRequest(t *testing.T) {
	t.Run("Success: CONNECT with multiple headers", func(t *testing.T) {
		input := "CONNECT example.com:443 HTTP/1.1\n" +
			"Host: example.com:443\n" +
			"User-Agent: curl/7.68.0\n" +
			"Proxy-Connection: Keep-Alive\n"

		reader := strings.NewReader(input)
		request, err := ReadProxyRequest(reader)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Verify Connect
		if request.Connect.HostPort.Host != "example.com" {
			t.Errorf("Expected host 'example.com', got %q", request.Connect.HostPort.Host)
		}
		if request.Connect.HostPort.Port != 443 {
			t.Errorf("Expected port 443, got %d", request.Connect.HostPort.Port)
		}
		if request.Connect.Version.Major != 1 || request.Connect.Version.Minor != 1 {
			t.Errorf("Expected HTTP/1.1, got HTTP/%d.%d", request.Connect.Version.Major, request.Connect.Version.Minor)
		}

		// Verify Headers
		expectedHeaders := []ast.Header{
			{Name: "Host", Value: "example.com:443"},
			{Name: "User-Agent", Value: "curl/7.68.0"},
			{Name: "Proxy-Connection", Value: "Keep-Alive"},
		}

		if len(request.Headers) != len(expectedHeaders) {
			t.Fatalf("Expected %d headers, got %d", len(expectedHeaders), len(request.Headers))
		}

		for i, h := range expectedHeaders {
			if request.Headers[i].Name != h.Name || request.Headers[i].Value != h.Value {
				t.Errorf("Header %d: expected %+v, got %+v", i, h, request.Headers[i])
			}
		}
	})

	t.Run("Success: Only CONNECT", func(t *testing.T) {
		input := "CONNECT example.com:80 HTTP/1.1\n"
		reader := strings.NewReader(input)
		request, err := ReadProxyRequest(reader)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if request.Connect.HostPort.Host != "example.com" || request.Connect.HostPort.Port != 80 {
			t.Errorf("Expected example.com:80, got %s:%d", request.Connect.HostPort.Host, request.Connect.HostPort.Port)
		}

		if len(request.Headers) != 0 {
			t.Errorf("Expected 0 headers, got %d", len(request.Headers))
		}
	})

	t.Run("Failure: Invalid CONNECT line", func(t *testing.T) {
		input := "GET example.com:443 HTTP/1.1\n"
		reader := strings.NewReader(input)
		_, err := ReadProxyRequest(reader)

		if err == nil {
			t.Fatal("Expected error for invalid CONNECT line, but got none")
		}
	})

	t.Run("Failure: Empty reader", func(t *testing.T) {
		input := ""
		reader := strings.NewReader(input)
		_, err := ReadProxyRequest(reader)

		if err == nil {
			t.Fatal("Expected error for empty reader, but got none")
		}
		if err.Error() != "failed to read connect message" {
			t.Errorf("Expected 'failed to read connect message' error, got %q", err.Error())
		}
	})

	t.Run("Success: Partial invalid headers should still return what was parsed", func(t *testing.T) {
		// Since readProxyRequest breaks on header parsing error, it should still return the CONNECT part
		input := "CONNECT example.com:443 HTTP/1.1\n" +
			"Host: example.com:443\n" +
			"InvalidHeader\n" +
			"Another: header\n"

		reader := strings.NewReader(input)
		request, err := ReadProxyRequest(reader)

		if err != nil {
			t.Fatalf("Expected no error (it breaks from loop), got %v", err)
		}

		if len(request.Headers) != 1 {
			t.Errorf("Expected 1 valid header before failure, got %d", len(request.Headers))
		}
		if request.Headers[0].Name != "Host" {
			t.Errorf("Expected 'Host' header, got %q", request.Headers[0].Name)
		}
	})

	t.Run("Success: Header with empty value", func(t *testing.T) {
		input := "CONNECT example.com:443 HTTP/1.1\n" +
			"Empty:\n" +
			"Another: value\n"

		reader := strings.NewReader(input)
		request, err := ReadProxyRequest(reader)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if len(request.Headers) != 2 {
			t.Fatalf("Expected 2 headers, got %d", len(request.Headers))
		}

		if request.Headers[0].Name != "Empty" || request.Headers[0].Value != "" {
			t.Errorf("Expected Empty header with empty value, got %+v", request.Headers[0])
		}
	})

	t.Run("Success: Host with no port", func(t *testing.T) {
		input := "CONNECT example.com HTTP/1.1\n" +
			"Cookie: user=toto\n"

		reader := strings.NewReader(input)
		request, err := ReadProxyRequest(reader)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if len(request.Headers) != 1 {
			t.Fatalf("Expected 1 headers, got %d", len(request.Headers))
		}

		if request.Headers[0].Name != "Cookie" || request.Headers[0].Value != "user=toto" {
			t.Errorf("Expected header, got %+v", request.Headers[0])
		}

		if request.Connect.HostPort.Port != 443 {
			t.Errorf("Expected port 443, got %d", request.Connect.HostPort.Port)
		}

		if request.Connect.HostPort.Host != "example.com" {
			t.Errorf("Expected host example.com, got %s", request.Connect.HostPort.Host)
		}
	})
}
