package composer

import (
	"httpStackLens/http/models"
	"testing"
)
import p "github.com/rflechner/EasyParsingForGo/combinator"

func TestHttpRequestLineParser(t *testing.T) {
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

	t.Run("Success: method, endpoint and version of a request line", func(t *testing.T) {
		cases := []struct {
			name         string
			input        string
			method       models.HttpMethod
			host         string
			port         int
			pathAndQuery string
			major        int
			minor        int
		}{
			{"lowercase verb", "get https://api.ipify.org HTTP/1.1",
				models.GET, "api.ipify.org", 443, "/", 1, 1},
			{"http scheme defaults to port 80", "GET http://example.com/a HTTP/1.1",
				models.GET, "example.com", 80, "/a", 1, 1},
			{"explicit port wins over the scheme default", "POST http://localhost:8080/api/items HTTP/1.1",
				models.POST, "localhost", 8080, "/api/items", 1, 1},
			{"HTTP/1.0", "HEAD https://example.com/a HTTP/1.0",
				models.HEAD, "example.com", 443, "/a", 1, 0},
			{"HTTP/2.0", "GET https://example.com/a HTTP/2.0",
				models.GET, "example.com", 443, "/a", 2, 0},
			{"QUERY verb", "QUERY https://api.example.com/search HTTP/1.1",
				models.QUERY, "api.example.com", 443, "/search", 1, 1},
			{"host only gets a root path", "DELETE https://example.com HTTP/1.1",
				models.DELETE, "example.com", 443, "/", 1, 1},
			{"userinfo is dropped, port and query are kept", "PUT https://user:pw@example.com:8443/a?b=1&c=2 HTTP/1.1",
				models.PUT, "example.com", 8443, "/a?b=1&c=2", 1, 1},
			{"extra spaces after the verb", "GET  https://example.com/a HTTP/1.1",
				models.GET, "example.com", 443, "/a", 1, 1},
			// A query with no path keeps no leading slash — the URL is echoed as written.
			{"query without a path", "GET https://api.ipify.org?format=json HTTP/1.1",
				models.GET, "api.ipify.org", 443, "?format=json", 1, 1},
		}

		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				result, err := HttpRequestLineParser()(p.NewParsingContext(c.input))
				if err != nil {
					t.Fatalf("Expected success, got error: %v", err)
				}
				if result.Result.HttpMethod != c.method {
					t.Errorf("Expected method %q, got %q", c.method, result.Result.HttpMethod)
				}
				if result.Result.Endpoint.Host != c.host {
					t.Errorf("Expected host %q, got %q", c.host, result.Result.Endpoint.Host)
				}
				if result.Result.Endpoint.Port != c.port {
					t.Errorf("Expected port %d, got %d", c.port, result.Result.Endpoint.Port)
				}
				if result.Result.Endpoint.PathAndQuery != c.pathAndQuery {
					t.Errorf("Expected path and query %q, got %q", c.pathAndQuery, result.Result.Endpoint.PathAndQuery)
				}
				if result.Result.Version.Major != c.major || result.Result.Version.Minor != c.minor {
					t.Errorf("Expected version %d.%d, got %d.%d",
						c.major, c.minor, result.Result.Version.Major, result.Result.Version.Minor)
				}
			})
		}
	})

	t.Run("Success: an unreadable version falls back to 1.1 and is left unconsumed", func(t *testing.T) {
		result, err := HttpRequestLineParser()(p.NewParsingContext("GET https://example.com/a HTTP/1"))
		if err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}
		if result.Result.Version.Major != 1 || result.Result.Version.Minor != 1 {
			t.Errorf("Expected the 1.1 default, got %d.%d", result.Result.Version.Major, result.Result.Version.Minor)
		}
		if remaining := string(result.Context.Remaining); remaining != " HTTP/1" {
			t.Errorf("Expected the malformed version to remain, got %q", remaining)
		}
	})

	t.Run("Success: parsing stops at the end of the request line", func(t *testing.T) {
		input := "GET https://api.ipify.org HTTP/1.1\nAccept: application/json\n"
		result, err := HttpRequestLineParser()(p.NewParsingContext(input))
		if err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}
		if remaining := string(result.Context.Remaining); remaining != "\nAccept: application/json\n" {
			t.Errorf("Expected the headers to remain, got %q", remaining)
		}
	})

	t.Run("Failure: unusable request lines", func(t *testing.T) {
		cases := []struct {
			name  string
			input string
		}{
			{"unknown verb", "FOO https://example.com/a HTTP/1.1"},
			{"origin-form target without a scheme", "GET /relative/path HTTP/1.1"},
			{"authority-form target", "CONNECT example.com:443 HTTP/1.1"},
			{"no target", "GET HTTP/1.1"},
			{"empty input", ""},
		}

		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				if _, err := HttpRequestLineParser()(p.NewParsingContext(c.input)); err == nil {
					t.Errorf("Expected an error for %q, got success", c.input)
				}
			})
		}
	})

	t.Run("Failure: the version is in practice mandatory", func(t *testing.T) {
		// p.Optional(VersionParser()) suggests otherwise, but the URL is read with
		// UntilText(..., " HTTP/"): without that marker the endpoint parser fails
		// before the optional version is ever reached. A `.http` file line such as
		// "GET https://api.ipify.org?format=json" therefore does not parse yet.
		for _, input := range []string{
			"GET https://api.ipify.org?format=json",
			"GET https://api.ipify.org?format=json\nAccept: application/json\n",
		} {
			if _, err := HttpRequestLineParser()(p.NewParsingContext(input)); err == nil {
				t.Errorf("Expected an error for %q, got success", input)
			}
		}
	})
}

func TestCommentLineParser(t *testing.T) {
	t.Run("Success: comment text is returned without its '#' markers", func(t *testing.T) {
		cases := []struct {
			name     string
			input    string
			expected string
		}{
			{"request separator", "### GET request example\nGET https://api.ipify.org\n", " GET request example"},
			{"single marker", "# enforcing IPv4\nGET https://api.ipify.org\n", " enforcing IPv4"},
			{"no space after the markers", "###enforcing IPv4\n", "enforcing IPv4"},
			{"empty comment", "###\nGET https://api.ipify.org\n", ""},
			{"empty line", "\nGET https://api.ipify.org\n", ""},
			{"markers inside the text are kept", "### issue #42 — see RFC #7230\n", " issue #42 — see RFC #7230"},
			{"trailing spaces are kept", "### spaced   \n", " spaced   "},
		}

		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				result, err := CommentLineParser()(p.NewParsingContext(c.input))
				if err != nil {
					t.Fatalf("Expected success, got error: %v", err)
				}
				if result.Result != c.expected {
					t.Errorf("Expected comment %q, got %q", c.expected, result.Result)
				}
			})
		}
	})

	t.Run("Success: parsing stops on the line break, leaving it to the caller", func(t *testing.T) {
		input := "### enforcing IPv4\nGET https://api.ipify.org\n"
		result, err := CommentLineParser()(p.NewParsingContext(input))
		if err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}

		remaining := string(result.Context.Remaining)
		if remaining != "\nGET https://api.ipify.org\n" {
			t.Errorf("Expected the line break and the rest of the file to remain, got %q", remaining)
		}
	})

	t.Run("Failure: a comment on the last line without a line break", func(t *testing.T) {
		// UntilText needs its delimiter: a file whose last line is a comment and
		// which does not end with a newline cannot be parsed.
		for _, input := range []string{"###", "### trailing comment", ""} {
			if _, err := CommentLineParser()(p.NewParsingContext(input)); err == nil {
				t.Errorf("Expected an error for %q, got success", input)
			}
		}
	})

	t.Run("Behaviour: the '#' markers are optional", func(t *testing.T) {
		// Many() never fails, so any line parses as a comment. Callers that need
		// to tell a comment from a request line must check the marker themselves.
		result, err := CommentLineParser()(p.NewParsingContext("GET https://api.ipify.org\n"))
		if err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}
		if result.Result != "GET https://api.ipify.org" {
			t.Errorf("Expected the whole line as comment text, got %q", result.Result)
		}
	})

	t.Run("Behaviour: CRLF files keep the carriage return in the comment", func(t *testing.T) {
		// The delimiter is "\n" alone, so on a Windows-authored .http file the
		// text ends with a stray '\r'.
		result, err := CommentLineParser()(p.NewParsingContext("### enforcing IPv4\r\nGET https://api.ipify.org\r\n"))
		if err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}
		if result.Result != " enforcing IPv4\r" {
			t.Errorf("Expected the carriage return to be kept, got %q", result.Result)
		}
	})
}
