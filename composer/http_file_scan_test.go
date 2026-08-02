package composer

import (
	_ "embed"
	"strings"
	"testing"

	"httpStackLens/http/models"

	p "github.com/rflechner/EasyParsingForGo/combinator"
)

//go:embed http_file_sample.http
var sampleFile string

// slice is what a span is for: the stretch of source it points at.
func slice(t *testing.T, source string, start, end p.TextPosition) string {
	t.Helper()
	runes := []rune(source)
	if start.Offset < 0 || end.Offset > len(runes) || start.Offset > end.Offset {
		t.Fatalf("span [%d,%d) is outside a %d rune source", start.Offset, end.Offset, len(runes))
	}
	return string(runes[start.Offset:end.Offset])
}

func TestSpans(t *testing.T) {
	t.Run("Success: the request line spans point at the tokens themselves", func(t *testing.T) {
		source := "GET  https://api.ipify.org?format=json HTTP/1.1\n"
		result, err := HttpRequestLineParser()(p.NewParsingContext(source))
		if err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}
		line := result.Result

		// The padding between the verb and the target belongs to neither.
		if got := slice(t, source, line.HttpMethod.Start, line.HttpMethod.End); got != "GET" {
			t.Errorf("Expected the method span to cover %q, got %q", "GET", got)
		}
		if got := slice(t, source, line.Target.Start, line.Target.End); got != "https://api.ipify.org?format=json" {
			t.Errorf("Unexpected target span %q", got)
		}
		if got := slice(t, source, line.Version.Start, line.Version.End); got != "HTTP/1.1" {
			t.Errorf("Expected the version span to cover %q, got %q", "HTTP/1.1", got)
		}
	})

	t.Run("Success: the target keeps the source text next to the resolved endpoint", func(t *testing.T) {
		result, err := HttpRequestLineParser()(p.NewParsingContext("GET https://example.com/a?b=1\n"))
		if err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}
		if !result.Result.Resolved {
			t.Error("Expected the target to be resolved")
		}
		if result.Result.Target.Text != "https://example.com/a?b=1" {
			t.Errorf("Unexpected target text %q", result.Result.Target.Text)
		}
		if result.Result.Endpoint.Text.Host != "example.com" {
			t.Errorf("Unexpected host %q", result.Result.Endpoint.Text.Host)
		}
	})

	t.Run("Success: an omitted version leaves an empty span on the 1.1 default", func(t *testing.T) {
		result, err := HttpRequestLineParser()(p.NewParsingContext("GET https://example.com/a\n"))
		if err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}
		if !result.Result.Version.Empty() {
			t.Error("Expected the version span to be empty, the version having been defaulted")
		}
		if result.Result.Version.Text.Major != 1 || result.Result.Version.Text.Minor != 1 {
			t.Errorf("Expected the 1.1 default, got %d.%d",
				result.Result.Version.Text.Major, result.Result.Version.Text.Minor)
		}
	})

	t.Run("Success: the comment span covers the marker the text drops", func(t *testing.T) {
		source := "# enforcing IPv4\n"
		result, err := CommentLineParser()(p.NewParsingContext(source))
		if err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}
		if got := slice(t, source, result.Result.Start, result.Result.End); got != "# enforcing IPv4" {
			t.Errorf("Expected the span to cover the whole line, got %q", got)
		}
		if result.Result.Text != " enforcing IPv4" {
			t.Errorf("Expected the text to drop the marker, got %q", result.Result.Text)
		}
	})

	t.Run("Success: a CRLF line leaves its carriage return out of the span", func(t *testing.T) {
		source := "# enforcing IPv4\r\n"
		result, err := CommentLineParser()(p.NewParsingContext(source))
		if err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}
		if got := slice(t, source, result.Result.Start, result.Result.End); got != "# enforcing IPv4" {
			t.Errorf("Expected the carriage return to be left out, got %q", got)
		}
	})

	t.Run("Success: the header name span stops before the colon", func(t *testing.T) {
		source := "Content-Type: application/json\n"
		result, err := positionedHeaderParser()(p.NewParsingContext(source))
		if err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}
		if got := slice(t, source, result.Result.Name.Start, result.Result.Name.End); got != "Content-Type" {
			t.Errorf("Expected the name span to stop before the colon, got %q", got)
		}
		if got := slice(t, source, result.Result.Value.Start, result.Result.Value.End); got != "application/json" {
			t.Errorf("Unexpected value span %q", got)
		}
	})

	t.Run("Success: the body span stops where the body does", func(t *testing.T) {
		source := "GET https://example.com/a\nContent-Type: text/plain\n\nfirst\n\nthird\n\n\n### Next\n"
		result, err := HttpRequestFileItemParser()(p.NewParsingContext(source))
		if err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}
		body := result.Result.Body
		if got := slice(t, source, body.Start, body.End); got != "first\n\nthird" {
			t.Errorf("Expected the blank lines before the separator to be left out, got %q", got)
		}
		if body.Text != "first\n\nthird" {
			t.Errorf("Unexpected body text %q", body.Text)
		}
	})
}

// A request target that still holds {{variables}} has no host to resolve until
// the file variables are substituted, and must survive parsing all the same.
func TestTemplateRequestTarget(t *testing.T) {
	t.Run("Success: a placeholder target is kept as written", func(t *testing.T) {
		source := "GET {{baseUrl}}/user HTTP/1.1\n"
		result, err := HttpRequestLineParser()(p.NewParsingContext(source))
		if err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}
		if result.Result.Resolved {
			t.Error("Expected the target to be left unresolved")
		}
		if result.Result.Target.Text != "{{baseUrl}}/user" {
			t.Errorf("Unexpected target %q", result.Result.Target.Text)
		}
		if result.Result.HttpMethod.Text != models.GET {
			t.Errorf("Expected GET, got %q", result.Result.HttpMethod.Text)
		}
		if result.Result.Version.Text.Major != 1 || result.Result.Version.Text.Minor != 1 {
			t.Errorf("Expected the version to still be read, got %d.%d",
				result.Result.Version.Text.Major, result.Result.Version.Text.Minor)
		}
	})

	t.Run("Failure: a target without a placeholder is still rejected", func(t *testing.T) {
		// The placeholder is what tells a template apart from a typo: without
		// one, an unusable target stays a parse error.
		for _, input := range []string{
			"GET /relative/path HTTP/1.1",
			"GET HTTP/1.1",
			"GET example.com HTTP/1.1",
		} {
			if _, err := HttpRequestLineParser()(p.NewParsingContext(input)); err == nil {
				t.Errorf("Expected an error for %q, got success", input)
			}
		}
	})

	t.Run("Success: a target does not run past its own line", func(t *testing.T) {
		// helpers.UrlParser looks for " HTTP/" anywhere ahead: without the line
		// boundary the first target would swallow the second request.
		source := "GET https://example.com/a\nGET https://example.com/b HTTP/1.1\n"
		result, err := HttpRequestLineParser()(p.NewParsingContext(source))
		if err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}
		if result.Result.Target.Text != "https://example.com/a" {
			t.Errorf("Expected the target to stop at the line break, got %q", result.Result.Target.Text)
		}
	})
}

func TestFileVariableParser(t *testing.T) {
	t.Run("Success: name and value", func(t *testing.T) {
		cases := []struct {
			name  string
			input string
			key   string
			value string
		}{
			{"spaces around the equals", "@baseUrl = https://api.github.com\n", "baseUrl", "https://api.github.com"},
			{"no spaces", "@token=ghp_example\n", "token", "ghp_example"},
			{"dotted name", "@api.v2.host = example.com\n", "api.v2.host", "example.com"},
			{"empty value", "@empty =\n", "empty", ""},
			{"value holding a placeholder", "@url = {{host}}/api\n", "url", "{{host}}/api"},
			{"last line without a break", "@token = abc", "token", "abc"},
		}

		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				result, err := FileVariableParser()(p.NewParsingContext(c.input))
				if err != nil {
					t.Fatalf("Expected success, got error: %v", err)
				}
				if result.Result.Name.Text != c.key {
					t.Errorf("Expected name %q, got %q", c.key, result.Result.Name.Text)
				}
				if result.Result.Value.Text != c.value {
					t.Errorf("Expected value %q, got %q", c.value, result.Result.Value.Text)
				}
			})
		}
	})

	t.Run("Success: the name span covers the marker", func(t *testing.T) {
		source := "@baseUrl = https://api.github.com\n"
		result, err := FileVariableParser()(p.NewParsingContext(source))
		if err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}
		if got := slice(t, source, result.Result.Name.Start, result.Result.Name.End); got != "@baseUrl" {
			t.Errorf("Expected the span to cover %q, got %q", "@baseUrl", got)
		}
	})

	t.Run("Failure: lines that are not variables", func(t *testing.T) {
		for _, input := range []string{
			"baseUrl = https://api.github.com\n",
			"@ = value\n",
			"@baseUrl https://api.github.com\n",
			"GET https://example.com\n",
			"",
		} {
			if _, err := FileVariableParser()(p.NewParsingContext(input)); err == nil {
				t.Errorf("Expected an error for %q, got success", input)
			}
		}
	})
}

// ParseHttpFile reads a whole file and, above all, keeps going: a block it
// cannot read costs that block and nothing else.
func TestParseHttpFile(t *testing.T) {
	t.Run("Success: variables then blocks", func(t *testing.T) {
		source := "@baseUrl = https://api.github.com\n" +
			"@token = ghp_example\n" +
			"\n" +
			"### Current user\n" +
			"GET {{baseUrl}}/user\n" +
			"Authorization: Bearer {{token}}\n" +
			"\n" +
			"### Create comment\n" +
			"POST {{baseUrl}}/comments\n" +
			"Content-Type: application/json\n" +
			"\n" +
			"{ \"body\": \"hi\" }\n"

		file := ParseHttpFile(source)

		if len(file.Issues) != 0 {
			t.Fatalf("Expected no issue, got %v", file.Issues)
		}
		if len(file.Variables) != 2 {
			t.Fatalf("Expected 2 variables, got %d", len(file.Variables))
		}
		if file.Variables[0].Name.Text != "baseUrl" || file.Variables[1].Value.Text != "ghp_example" {
			t.Errorf("Unexpected variables %+v", file.Variables)
		}
		if len(file.Requests) != 2 {
			t.Fatalf("Expected 2 requests, got %d", len(file.Requests))
		}
		if file.Requests[1].Body.Text != "{ \"body\": \"hi\" }" {
			t.Errorf("Unexpected body %q", file.Requests[1].Body.Text)
		}
	})

	t.Run("Success: a broken block does not cost the others", func(t *testing.T) {
		source := "### First\n" +
			"GET https://example.com/a\n" +
			"\n" +
			"### Broken\n" +
			"GE https://example.com/b\n" +
			"\n" +
			"### Third\n" +
			"GET https://example.com/c\n"

		file := ParseHttpFile(source)

		if len(file.Requests) != 2 {
			t.Fatalf("Expected the 2 readable requests, got %d", len(file.Requests))
		}
		if file.Requests[0].Body.Text != "" || file.Requests[1].HttpRequestLine.Target.Text != "https://example.com/c" {
			t.Errorf("Expected the first and third blocks, got %+v", file.Requests)
		}
		if len(file.Issues) != 1 {
			t.Fatalf("Expected 1 issue, got %d: %v", len(file.Issues), file.Issues)
		}
		if got := slice(t, source, file.Issues[0].Start, file.Issues[0].End); got != "### Broken\nGE https://example.com/b" {
			t.Errorf("Expected the issue to span the broken block, got %q", got)
		}
	})

	t.Run("Success: an empty or blank file yields nothing at all", func(t *testing.T) {
		for _, source := range []string{"", "\n", "   \n\n\t\n"} {
			file := ParseHttpFile(source)
			if len(file.Requests) != 0 || len(file.Variables) != 0 || len(file.Issues) != 0 {
				t.Errorf("Expected an empty file for %q, got %+v", source, file)
			}
		}
	})

	t.Run("Success: a file of comments alone is reported, not dropped", func(t *testing.T) {
		file := ParseHttpFile("### Nothing below yet\n")
		if len(file.Requests) != 0 {
			t.Errorf("Expected no request, got %d", len(file.Requests))
		}
		if len(file.Issues) != 1 {
			t.Fatalf("Expected the block to be reported, got %d issues", len(file.Issues))
		}
	})

	t.Run("Success: the sample file parses whole", func(t *testing.T) {
		source := strings.ReplaceAll(sampleFile, "\r\n", "\n")
		file := ParseHttpFile(source)
		if len(file.Requests) == 0 {
			t.Fatal("Expected the sample file to yield requests")
		}
	})
}
