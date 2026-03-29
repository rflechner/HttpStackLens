package parser

import (
	"testing"
)
import p "github.com/rflechner/EasyParsingForGo/combinator"

func TestConnectParser(t *testing.T) {
	t.Run("Success: Standard CONNECT", func(t *testing.T) {
		input := "CONNECT example.com:443 HTTP/1.1"
		context := p.NewParsingContext(input)
		parser := ConnectParser()

		result, err := parser(context)
		if err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}

		if result.Result.HostPort.Host != "example.com" {
			t.Errorf("Expected host 'example.com', got %q", result.Result.HostPort.Host)
		}
		if result.Result.HostPort.Port != 443 {
			t.Errorf("Expected port 443, got %d", result.Result.HostPort.Port)
		}
		if result.Result.Version.Major != 1 || result.Result.Version.Minor != 1 {
			t.Errorf("Expected version 1.1, got %d.%d", result.Result.Version.Major, result.Result.Version.Minor)
		}
	})

	t.Run("Success: Standard CONNECT", func(t *testing.T) {
		input := "CONNECT https://www.youtube.com/ HTTP/1.1"
		context := p.NewParsingContext(input)
		parser := ConnectParser()

		result, err := parser(context)
		if err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}

		if result.Result.HostPort.Host != "www.youtube.com" {
			t.Errorf("Expected host 'www.youtube.com', got %q", result.Result.HostPort.Host)
		}
		if result.Result.HostPort.Port != 443 {
			t.Errorf("Expected port 443, got %d", result.Result.HostPort.Port)
		}
		if result.Result.Version.Major != 1 || result.Result.Version.Minor != 1 {
			t.Errorf("Expected version 1.1, got %d.%d", result.Result.Version.Major, result.Result.Version.Minor)
		}
	})

	t.Run("Success: Missing port has default port 443", func(t *testing.T) {
		input := "CONNECT example.com HTTP/1.1"
		context := p.NewParsingContext(input)
		parser := ConnectParser()

		result, err := parser(context)
		if err != nil {
			t.Fatalf("Connect parsing got error %s", err.Error())
		}

		if result.Result.HostPort.Host != "example.com" {
			t.Fatalf("Expected host 'example.com', got %q", result.Result.HostPort.Host)
		}

		if result.Result.HostPort.Port != 443 {
			t.Fatalf("Expected port 443, got %d", result.Result.HostPort.Port)
		}
	})

	t.Run("Failure: Invalid command", func(t *testing.T) {
		input := "GET example.com:443 HTTP/1.1"
		context := p.NewParsingContext(input)
		parser := ConnectParser()

		_, err := parser(context)
		if err == nil {
			t.Fatal("Expected error due to invalid command, but got success")
		}
	})

	t.Run("Failure: Invalid HTTP version", func(t *testing.T) {
		input := "CONNECT example.com:443 HTTP/INVALID"
		context := p.NewParsingContext(input)
		parser := ConnectParser()

		_, err := parser(context)
		if err == nil {
			t.Fatal("Expected error due to invalid HTTP version, but got success")
		}
	})
}

func TestHTTPVersionParser(t *testing.T) {
	t.Run("Success: HTTP/1.1", func(t *testing.T) {
		input := "HTTP/1.1"
		context := p.NewParsingContext(input)
		parser := VersionParser()

		result, err := parser(context)
		if err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}

		if result.Result.Major != 1 || result.Result.Minor != 1 {
			t.Errorf("Expected 1.1, got %d.%d", result.Result.Major, result.Result.Minor)
		}
	})

	t.Run("Success: HTTP/2.0", func(t *testing.T) {
		input := "HTTP/2.0"
		context := p.NewParsingContext(input)
		parser := VersionParser()

		result, err := parser(context)
		if err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}

		if result.Result.Major != 2 || result.Result.Minor != 0 {
			t.Errorf("Expected 2.0, got %d.%d", result.Result.Major, result.Result.Minor)
		}
	})
}

func TestResponseHeadParser(t *testing.T) {
	t.Run("Success: Simple OK", func(t *testing.T) {
		input := "HTTP/1.1 200 OK\r\n\r\n"
		context := p.NewParsingContext(input)
		parser := ResponseHeadParser()

		result, err := parser(context)
		if err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}

		if result.Result.StatusCode != 200 {
			t.Errorf("Expected status code 200, got %d", result.Result.StatusCode)
		}
		if result.Result.StatusDescription != "OK" {
			t.Errorf("Expected status description 'OK', got %q", result.Result.StatusDescription)
		}
		if len(result.Result.Headers) != 0 {
			t.Errorf("Expected 0 headers, got %d", len(result.Result.Headers))
		}
	})

	t.Run("Success: OK with headers", func(t *testing.T) {
		input := "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 42\r\n\r\n"
		context := p.NewParsingContext(input)
		parser := ResponseHeadParser()

		result, err := parser(context)
		if err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}

		if len(result.Result.Headers) != 2 {
			t.Errorf("Expected 2 headers, got %d", len(result.Result.Headers))
		}
		if len(result.Result.GetHeader("Content-Type")) == 0 || result.Result.GetHeader("Content-Type")[0] != "application/json" {
			t.Errorf("Expected Content-Type: application/json, got %q", result.Result.GetHeader("Content-Type"))
		}
	})

	t.Run("Success: Multiple spaces in status description", func(t *testing.T) {
		input := "HTTP/1.1 404 Not Found Extended\r\n\r\n"
		context := p.NewParsingContext(input)
		parser := ResponseHeadParser()

		result, err := parser(context)
		if err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}

		if result.Result.StatusDescription != "Not Found Extended" {
			t.Errorf("Expected status description 'Not Found Extended', got %q", result.Result.StatusDescription)
		}
	})

	t.Run("Failure: Invalid Status Code", func(t *testing.T) {
		input := "HTTP/1.1 OK 200\r\n\r\n"
		context := p.NewParsingContext(input)
		parser := ResponseHeadParser()

		_, err := parser(context)
		if err == nil {
			t.Fatal("Expected error due to invalid status code, but got success")
		}
	})
}
