package http

import (
	"httpStackLens/http/models"
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
		if request.HttpRequestLine.Endpoint.Host != "example.com" {
			t.Errorf("Expected host 'example.com', got %q", request.HttpRequestLine.Endpoint.Host)
		}
		if request.HttpRequestLine.Endpoint.Port != 443 {
			t.Errorf("Expected port 443, got %d", request.HttpRequestLine.Endpoint.Port)
		}
		if request.HttpRequestLine.Version.Major != 1 || request.HttpRequestLine.Version.Minor != 1 {
			t.Errorf("Expected HTTP/1.1, got HTTP/%d.%d", request.HttpRequestLine.Version.Major, request.HttpRequestLine.Version.Minor)
		}

		// Verify Headers
		expectedHeaders := []models.Header{
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

		if request.HttpRequestLine.Endpoint.Host != "example.com" || request.HttpRequestLine.Endpoint.Port != 80 {
			t.Errorf("Expected example.com:80, got %s:%d", request.HttpRequestLine.Endpoint.Host, request.HttpRequestLine.Endpoint.Port)
		}

		if len(request.Headers) != 0 {
			t.Errorf("Expected 0 headers, got %d", len(request.Headers))
		}
	})

	t.Run("Success: GET request line", func(t *testing.T) {
		input := "GET example.com:443 HTTP/1.1\n"
		reader := strings.NewReader(input)
		result, err := ReadProxyRequest(reader)

		if err != nil {
			t.Fatal("Expected error for invalid CONNECT line, but got none")
		}

		if result.HttpRequestLine.Endpoint.Host != "example.com" || result.HttpRequestLine.Endpoint.Port != 443 {
			t.Errorf("Expected example.com:443, got %s:%d", result.HttpRequestLine.Endpoint.Host, result.HttpRequestLine.Endpoint.Port)
		}
		if result.HttpRequestLine.HttpMethod != "GET" {
			t.Errorf("Expected GET method, got %s", result.HttpRequestLine.HttpMethod)
		}
	})

	t.Run("Failure: Empty reader", func(t *testing.T) {
		input := ""
		reader := strings.NewReader(input)
		_, err := ReadProxyRequest(reader)

		if err == nil {
			t.Fatal("Expected error for empty reader, but got none")
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

		if request.HttpRequestLine.Endpoint.Port != 443 {
			t.Errorf("Expected port 443, got %d", request.HttpRequestLine.Endpoint.Port)
		}

		if request.HttpRequestLine.Endpoint.Host != "example.com" {
			t.Errorf("Expected host example.com, got %s", request.HttpRequestLine.Endpoint.Host)
		}
	})

	t.Run("Success: Clean HTTP GET request having a path and query parameters", func(t *testing.T) {
		input := "GET http://www.example.com/dir1/dir2/page.html?popo=123 HTTP/1.1\n" +
			"Host: www.example.com\n" +
			"User-Agent: Mozilla/5.0\n" +
			"Accept: text/html\n" +
			"Proxy-Authorization: Basic dXNlcjpwYXNz\n" +
			"Connection: keep-alive\n" +
			"\n"
		reader := strings.NewReader(input)
		result, err := ReadProxyRequest(reader)

		if err != nil {
			t.Fatalf("Expected no error, but got: %v", err)
		}

		if result.HttpRequestLine.Endpoint.Host != "www.example.com" || result.HttpRequestLine.Endpoint.Port != 80 {
			t.Errorf("Expected www.example.com:80, got %s:%d", result.HttpRequestLine.Endpoint.Host, result.HttpRequestLine.Endpoint.Port)
		}
		if result.HttpRequestLine.HttpMethod != "GET" {
			t.Errorf("Expected GET method, got %s", result.HttpRequestLine.HttpMethod)
		}
		if result.HttpRequestLine.Endpoint.PathAndQuery != "/dir1/dir2/page.html?popo=123" {
			t.Errorf("Expected path /dir1/dir2/page.html?popo=123, got %s", result.HttpRequestLine.Endpoint.PathAndQuery)
		}
	})
}
