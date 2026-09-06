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

		if result.Result.Endpoint.Text.Host != "api.ipify.org" {
			t.Errorf("Expected host 'api.ipify.org', got %q", result.Result.Endpoint.Text.Host)
		}
		if result.Result.Endpoint.Text.Port != 443 {
			t.Errorf("Expected port 443, got %d", result.Result.Endpoint.Text.Port)
		}
		if result.Result.Version.Text.Major != 1 || result.Result.Version.Text.Minor != 1 {
			t.Errorf("Expected version 1.1, got %d.%d", result.Result.Version.Text.Major, result.Result.Version.Text.Minor)
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
				if result.Result.HttpMethod.Text != c.method {
					t.Errorf("Expected method %q, got %q", c.method, result.Result.HttpMethod.Text)
				}
				if result.Result.Endpoint.Text.Host != c.host {
					t.Errorf("Expected host %q, got %q", c.host, result.Result.Endpoint.Text.Host)
				}
				if result.Result.Endpoint.Text.Port != c.port {
					t.Errorf("Expected port %d, got %d", c.port, result.Result.Endpoint.Text.Port)
				}
				if result.Result.Endpoint.Text.PathAndQuery != c.pathAndQuery {
					t.Errorf("Expected path and query %q, got %q", c.pathAndQuery, result.Result.Endpoint.Text.PathAndQuery)
				}
				if result.Result.Version.Text.Major != c.major || result.Result.Version.Text.Minor != c.minor {
					t.Errorf("Expected version %d.%d, got %d.%d",
						c.major, c.minor, result.Result.Version.Text.Major, result.Result.Version.Text.Minor)
				}
			})
		}
	})

	t.Run("Success: an unreadable version falls back to 1.1 and is left unconsumed", func(t *testing.T) {
		result, err := HttpRequestLineParser()(p.NewParsingContext("GET https://example.com/a HTTP/1"))
		if err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}
		if result.Result.Version.Text.Major != 1 || result.Result.Version.Text.Minor != 1 {
			t.Errorf("Expected the 1.1 default, got %d.%d", result.Result.Version.Text.Major, result.Result.Version.Text.Minor)
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

	t.Run("Success: a request line without a version falls back to 1.1", func(t *testing.T) {
		cases := []struct {
			name      string
			input     string
			remaining string
		}{
			{"end of input", "GET https://api.ipify.org?format=json", ""},
			{"end of line", "GET https://api.ipify.org?format=json\nAccept: application/json\n", "\nAccept: application/json\n"},
			{"CRLF end of line", "GET https://api.ipify.org?format=json\r\nAccept: application/json\r\n", "\nAccept: application/json\r\n"},
		}

		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				result, err := HttpRequestLineParser()(p.NewParsingContext(c.input))
				if err != nil {
					t.Fatalf("Expected success, got error: %v", err)
				}
				if result.Result.Endpoint.Text.Host != "api.ipify.org" {
					t.Errorf("Expected host 'api.ipify.org', got %q", result.Result.Endpoint.Text.Host)
				}
				if result.Result.Endpoint.Text.PathAndQuery != "?format=json" {
					t.Errorf("Expected path and query '?format=json', got %q", result.Result.Endpoint.Text.PathAndQuery)
				}
				if result.Result.Version.Text.Major != 1 || result.Result.Version.Text.Minor != 1 {
					t.Errorf("Expected the 1.1 default, got %d.%d",
						result.Result.Version.Text.Major, result.Result.Version.Text.Minor)
				}
				if remaining := string(result.Context.Remaining); remaining != c.remaining {
					t.Errorf("Expected %q to remain, got %q", c.remaining, remaining)
				}
			})
		}
	})
}

// CommentLineParser reads an ordinary comment line — a commented-out header, a
// note — introduced by a single '#'.
func TestCommentLineParser(t *testing.T) {
	t.Run("Success: text after the '#' marker", func(t *testing.T) {
		cases := []struct {
			name     string
			input    string
			expected string
		}{
			{"note", "# enforcing IPv4\nGET https://api.ipify.org\n", " enforcing IPv4"},
			{"commented-out header", "# Accept: application/json\n", " Accept: application/json"},
			{"no space after the marker", "#enforcing IPv4\n", "enforcing IPv4"},
			{"empty comment", "#\nGET https://api.ipify.org\n", ""},
			{"markers inside the text are kept", "# issue #42 — see RFC #7230\n", " issue #42 — see RFC #7230"},
			{"trailing spaces are kept", "# spaced   \n", " spaced   "},
		}

		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				result, err := CommentLineParser()(p.NewParsingContext(c.input))
				if err != nil {
					t.Fatalf("Expected success, got error: %v", err)
				}
				if result.Result.Text != c.expected {
					t.Errorf("Expected comment %q, got %q", c.expected, result.Result.Text)
				}
			})
		}
	})

	t.Run("Success: parsing stops on the line break, leaving it to the caller", func(t *testing.T) {
		input := "# enforcing IPv4\nGET https://api.ipify.org\n"
		result, err := CommentLineParser()(p.NewParsingContext(input))
		if err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}

		remaining := string(result.Context.Remaining)
		if remaining != "\nGET https://api.ipify.org\n" {
			t.Errorf("Expected the line break and the rest of the file to remain, got %q", remaining)
		}
	})

	t.Run("Failure: lines that are not comments", func(t *testing.T) {
		cases := []struct {
			name  string
			input string
		}{
			{"request line", "GET https://api.ipify.org HTTP/1.1\n"},
			{"header", "Accept: application/json\n"},
			{"empty line", "\nGET https://api.ipify.org\n"},
			{"marker not at the start of the line", "  # indented\n"},
			{"empty input", ""},
		}

		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				if _, err := CommentLineParser()(p.NewParsingContext(c.input)); err == nil {
					t.Errorf("Expected an error for %q, got success", c.input)
				}
			})
		}
	})

	t.Run("Failure: a comment on the last line without a line break", func(t *testing.T) {
		// UntilText needs its delimiter: a file whose last line is a comment and
		// which does not end with a newline cannot be parsed.
		for _, input := range []string{"#", "# trailing comment"} {
			if _, err := CommentLineParser()(p.NewParsingContext(input)); err == nil {
				t.Errorf("Expected an error for %q, got success", input)
			}
		}
	})

	t.Run("Behaviour: a '###' separator also parses, extra markers included", func(t *testing.T) {
		// '#' is a prefix of '###', so a request separator is a valid comment line
		// whose text starts with the two remaining markers. A file scanner must
		// therefore try HeaderCommentsParser first.
		result, err := CommentLineParser()(p.NewParsingContext("### Current user\nGET https://api.ipify.org\n"))
		if err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}
		if result.Result.Text != "## Current user" {
			t.Errorf("Expected the extra markers in the text, got %q", result.Result.Text)
		}
	})

	t.Run("Behaviour: CRLF files keep the carriage return in the comment", func(t *testing.T) {
		// The delimiter is "\n" alone, so on a Windows-authored .http file the
		// text ends with a stray '\r'.
		result, err := CommentLineParser()(p.NewParsingContext("# enforcing IPv4\r\nGET https://api.ipify.org\r\n"))
		if err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}
		if result.Result.Text != " enforcing IPv4\r" {
			t.Errorf("Expected the carriage return to be kept, got %q", result.Result.Text)
		}
	})
}

// HeaderCommentsParser reads the '###' line that opens a request block.
func TestHeaderCommentsParser(t *testing.T) {
	t.Run("Success: text after the '###' marker", func(t *testing.T) {
		cases := []struct {
			name     string
			input    string
			expected string
		}{
			{"named request", "### GET request example\nGET https://api.ipify.org\n", " GET request example"},
			{"bare separator", "###\nGET https://api.ipify.org\n", ""},
			{"no space after the marker", "###enforcing IPv4\n", "enforcing IPv4"},
			{"extra markers land in the text", "#### four markers\n", "# four markers"},
			{"markers inside the text are kept", "### issue #42 — see RFC #7230\n", " issue #42 — see RFC #7230"},
			{"trailing spaces are kept", "### spaced   \n", " spaced   "},
		}

		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				result, err := HeaderCommentsParser()(p.NewParsingContext(c.input))
				if err != nil {
					t.Fatalf("Expected success, got error: %v", err)
				}
				if result.Result.Text != c.expected {
					t.Errorf("Expected comment %q, got %q", c.expected, result.Result.Text)
				}
			})
		}
	})

	t.Run("Success: parsing stops on the line break, leaving it to the caller", func(t *testing.T) {
		input := "### GET request example\nGET https://api.ipify.org\n"
		result, err := HeaderCommentsParser()(p.NewParsingContext(input))
		if err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}

		remaining := string(result.Context.Remaining)
		if remaining != "\nGET https://api.ipify.org\n" {
			t.Errorf("Expected the line break and the rest of the file to remain, got %q", remaining)
		}
	})

	t.Run("Failure: fewer than three markers", func(t *testing.T) {
		// This is the whole point of the two parsers: a commented-out header must
		// not be mistaken for the separator that opens a request block.
		cases := []struct {
			name  string
			input string
		}{
			{"one marker", "# enforcing IPv4\n"},
			{"two markers", "## enforcing IPv4\n"},
			{"commented-out header", "# Accept: application/json\n"},
			{"request line", "GET https://api.ipify.org HTTP/1.1\n"},
			{"empty line", "\n"},
			{"marker not at the start of the line", "  ### indented\n"},
			{"empty input", ""},
		}

		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				if _, err := HeaderCommentsParser()(p.NewParsingContext(c.input)); err == nil {
					t.Errorf("Expected an error for %q, got success", c.input)
				}
			})
		}
	})

	t.Run("Failure: a separator on the last line without a line break", func(t *testing.T) {
		// http_file_sample.http ends with a bare "###": the file must end with a
		// newline for its last separator to be readable.
		for _, input := range []string{"###", "### trailing separator"} {
			if _, err := HeaderCommentsParser()(p.NewParsingContext(input)); err == nil {
				t.Errorf("Expected an error for %q, got success", input)
			}
		}
	})

	t.Run("Behaviour: CRLF files keep the carriage return in the comment", func(t *testing.T) {
		result, err := HeaderCommentsParser()(p.NewParsingContext("### GET request example\r\nGET https://api.ipify.org\r\n"))
		if err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}
		if result.Result.Text != " GET request example\r" {
			t.Errorf("Expected the carriage return to be kept, got %q", result.Result.Text)
		}
	})
}

// HttpRequestFileItemParser reads one "###" block: its comments, its request
// line, its headers and its body.
func TestHttpRequestFileItemParser(t *testing.T) {
	t.Run("Success: a complete block", func(t *testing.T) {
		input := "### Create issue comment\n" +
			"# the token comes from the file variables\n" +
			"POST https://api.github.com/repos/golang/go/issues/1/comments HTTP/1.1\n" +
			"Authorization: Bearer token\n" +
			"Content-Type: application/json\n" +
			"\n" +
			"{\n  \"body\": \"Reproduced on go1.23.2\"\n}\n" +
			"\n" +
			"### Next request\n" +
			"GET https://example.com\n"

		result, err := HttpRequestFileItemParser()(p.NewParsingContext(input))
		if err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}
		item := result.Result

		expectComments(t, item.HeaderComments, " Create issue comment", " the token comes from the file variables")

		if item.HttpRequestLine.HttpMethod.Text != models.POST {
			t.Errorf("Expected method POST, got %q", item.HttpRequestLine.HttpMethod.Text)
		}
		if item.HttpRequestLine.Endpoint.Text.Host != "api.github.com" {
			t.Errorf("Expected host 'api.github.com', got %q", item.HttpRequestLine.Endpoint.Text.Host)
		}
		if item.HttpRequestLine.Endpoint.Text.PathAndQuery != "/repos/golang/go/issues/1/comments" {
			t.Errorf("Unexpected path and query %q", item.HttpRequestLine.Endpoint.Text.PathAndQuery)
		}

		expectHeaders(t, item.Headers, "Authorization: Bearer token", "Content-Type: application/json")

		if item.Body.Text != "{\n  \"body\": \"Reproduced on go1.23.2\"\n}" {
			t.Errorf("Unexpected body %q", item.Body.Text)
		}
		if remaining := string(result.Context.Remaining); remaining != "### Next request\nGET https://example.com\n" {
			t.Errorf("Expected the next block to remain, got %q", remaining)
		}
	})

	t.Run("Success: a request line without a version", func(t *testing.T) {
		input := "### GET request example for calling API of ipify.org\n" +
			"### enforcing IPv4\n" +
			"GET https://api.ipify.org?format=json\n" +
			"Accept: application/json\n"

		result, err := HttpRequestFileItemParser()(p.NewParsingContext(input))
		if err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}
		item := result.Result

		expectComments(t, item.HeaderComments,
			" GET request example for calling API of ipify.org", " enforcing IPv4")
		if item.HttpRequestLine.Version.Text.Major != 1 || item.HttpRequestLine.Version.Text.Minor != 1 {
			t.Errorf("Expected the 1.1 default, got %d.%d",
				item.HttpRequestLine.Version.Text.Major, item.HttpRequestLine.Version.Text.Minor)
		}
		if item.HttpRequestLine.Endpoint.Text.PathAndQuery != "?format=json" {
			t.Errorf("Unexpected path and query %q", item.HttpRequestLine.Endpoint.Text.PathAndQuery)
		}
		expectHeaders(t, item.Headers, "Accept: application/json")
		if item.Body.Text != "" {
			t.Errorf("Expected no body, got %q", item.Body.Text)
		}
		if !result.Context.AtEnd() {
			t.Errorf("Expected the whole input to be consumed, got %q", string(result.Context.Remaining))
		}
	})

	t.Run("Success: a block without comments nor headers", func(t *testing.T) {
		result, err := HttpRequestFileItemParser()(p.NewParsingContext("GET https://example.com/a HTTP/1.1\n"))
		if err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}
		if len(result.Result.HeaderComments) != 0 {
			t.Errorf("Expected no comment, got %d", len(result.Result.HeaderComments))
		}
		if len(result.Result.Headers) != 0 {
			t.Errorf("Expected no header, got %d", len(result.Result.Headers))
		}
		if result.Result.Body.Text != "" {
			t.Errorf("Expected no body, got %q", result.Result.Body.Text)
		}
	})

	t.Run("Success: comments between the headers are collected too", func(t *testing.T) {
		input := "### Current user\n" +
			"GET https://api.github.com/user HTTP/1.1\n" +
			"Accept: application/json\n" +
			"# Authorization: Bearer disabled-for-now\n" +
			"User-Agent: httpStackLens\n"

		result, err := HttpRequestFileItemParser()(p.NewParsingContext(input))
		if err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}

		expectComments(t, result.Result.HeaderComments,
			" Current user", " Authorization: Bearer disabled-for-now")
		expectHeaders(t, result.Result.Headers,
			"Accept: application/json", "User-Agent: httpStackLens")
	})

	t.Run("Success: leading blank lines are skipped", func(t *testing.T) {
		input := "\n\n### Current user\nGET https://api.github.com/user HTTP/1.1\n"
		result, err := HttpRequestFileItemParser()(p.NewParsingContext(input))
		if err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}
		expectComments(t, result.Result.HeaderComments, " Current user")
	})

	t.Run("Success: the body keeps its inner blank lines", func(t *testing.T) {
		input := "GET https://example.com/a HTTP/1.1\n" +
			"Content-Type: text/plain\n" +
			"\n" +
			"first\n" +
			"\n" +
			"third\n" +
			"\n" +
			"\n" +
			"### Next request\n"

		result, err := HttpRequestFileItemParser()(p.NewParsingContext(input))
		if err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}
		if result.Result.Body.Text != "first\n\nthird" {
			t.Errorf("Unexpected body %q", result.Result.Body.Text)
		}
		if remaining := string(result.Context.Remaining); remaining != "### Next request\n" {
			t.Errorf("Expected the next block to remain, got %q", remaining)
		}
	})

	t.Run("Success: a CRLF block", func(t *testing.T) {
		input := "### Create issue comment\r\n" +
			"POST https://api.github.com/issues HTTP/1.1\r\n" +
			"Content-Type: application/json\r\n" +
			"\r\n" +
			"{\r\n  \"body\": \"hello\"\r\n}\r\n"

		result, err := HttpRequestFileItemParser()(p.NewParsingContext(input))
		if err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}
		expectHeaders(t, result.Result.Headers, "Content-Type: application/json")
		if result.Result.Body.Text != "{\r\n  \"body\": \"hello\"\r\n}" {
			t.Errorf("Unexpected body %q", result.Result.Body.Text)
		}
	})

	t.Run("Success: consecutive blocks can be read in a loop", func(t *testing.T) {
		input := "### First\n" +
			"GET https://example.com/a\n" +
			"\n" +
			"### Second\n" +
			"POST https://example.com/b\n" +
			"Content-Type: text/plain\n" +
			"\n" +
			"payload\n"

		context := p.NewParsingContext(input)
		var items []HttpRequestFileItem
		for !context.AtEnd() {
			result, err := HttpRequestFileItemParser()(context)
			if err != nil {
				t.Fatalf("Expected success, got error: %v", err)
			}
			items = append(items, result.Result)
			context = result.Context
		}

		if len(items) != 2 {
			t.Fatalf("Expected 2 items, got %d", len(items))
		}
		expectComments(t, items[0].HeaderComments, " First")
		expectComments(t, items[1].HeaderComments, " Second")
		if items[1].Body.Text != "payload" {
			t.Errorf("Unexpected body %q", items[1].Body.Text)
		}
	})

	t.Run("Failure: a block without a request line", func(t *testing.T) {
		cases := []struct {
			name  string
			input string
		}{
			{"comments only", "### Current user\n# nothing below\n"},
			{"dangling separator", "###"},
			{"headers only", "Accept: application/json\n"},
			{"empty input", ""},
		}

		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				if _, err := HttpRequestFileItemParser()(p.NewParsingContext(c.input)); err == nil {
					t.Errorf("Expected an error for %q, got success", c.input)
				}
			})
		}
	})
}

func expectComments(t *testing.T, comments []CommentLine, expected ...string) {
	t.Helper()
	if len(comments) != len(expected) {
		t.Fatalf("Expected %d comments, got %d: %v", len(expected), len(comments), commentTexts(comments))
	}
	for i, want := range expected {
		if comments[i].Text != want {
			t.Errorf("Expected comment %d to be %q, got %q", i, want, comments[i].Text)
		}
	}
}

func commentTexts(comments []CommentLine) []string {
	texts := make([]string, 0, len(comments))
	for _, comment := range comments {
		texts = append(texts, comment.Text)
	}
	return texts
}

// expectHeaders compares the parsed headers with "Name: value" strings.
func expectHeaders(t *testing.T, headers []PositionedHeader, expected ...string) {
	t.Helper()
	if len(headers) != len(expected) {
		t.Fatalf("Expected %d headers, got %d: %v", len(expected), len(headers), headerTexts(headers))
	}
	for i, want := range expected {
		if got := headers[i].Name.Text + ": " + headers[i].Value.Text; got != want {
			t.Errorf("Expected header %d to be %q, got %q", i, want, got)
		}
	}
}

func headerTexts(headers []PositionedHeader) []string {
	texts := make([]string, 0, len(headers))
	for _, header := range headers {
		texts = append(texts, header.Name.Text+": "+header.Value.Text)
	}
	return texts
}
