package composer

import (
	"testing"
)
import p "github.com/rflechner/EasyParsingForGo/combinator"

func TestConnectParser(t *testing.T) {
	t.Run("Success: Http file request line with a version", func(t *testing.T) {
		input := "GET https://api.ipify.org?format=json HTTP/1.1"
		context := p.NewParsingContext(input)
		parser := HttpRequestLineParser()

		result, err := parser(context)
		if err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}

		if result.Result.Endpoint.Host != "api.ipify.org" {
			t.Errorf("Expected host 'api.ipify.org', got %q", result.Result.Endpoint.Host)
		}
		if result.Result.Endpoint.Port != 443 {
			t.Errorf("Expected port 443, got %d", result.Result.Endpoint.Port)
		}
		if result.Result.Version.Major != 1 || result.Result.Version.Minor != 1 {
			t.Errorf("Expected version 1.1, got %d.%d", result.Result.Version.Major, result.Result.Version.Minor)
		}
	})
}
