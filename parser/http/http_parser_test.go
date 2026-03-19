package http

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

		if result.Result.Host != "example.com" {
			t.Errorf("Expected host 'example.com', got %q", result.Result.Host)
		}
		if result.Result.Port != 443 {
			t.Errorf("Expected port 443, got %d", result.Result.Port)
		}
		if result.Result.Version.Major != 1 || result.Result.Version.Minor != 1 {
			t.Errorf("Expected version 1.1, got %d.%d", result.Result.Version.Major, result.Result.Version.Minor)
		}
	})

	t.Run("Failure: Missing port", func(t *testing.T) {
		input := "CONNECT example.com HTTP/1.1"
		context := p.NewParsingContext(input)
		parser := ConnectParser()

		_, err := parser(context)
		if err == nil {
			t.Fatal("Expected error due to missing port, but got success")
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
		parser := HTTPVersionParser()

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
		parser := HTTPVersionParser()

		result, err := parser(context)
		if err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}

		if result.Result.Major != 2 || result.Result.Minor != 0 {
			t.Errorf("Expected 2.0, got %d.%d", result.Result.Major, result.Result.Minor)
		}
	})
}
