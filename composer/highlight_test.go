package composer

import (
	"regexp"
	"strings"
	"testing"
)

// texts renders the tokens back as the source stretches they point at, which is
// what a test can read.
func texts(source string, tokens []Token) []string {
	runes := []rune(source)
	out := make([]string, 0, len(tokens))
	for _, token := range tokens {
		out = append(out, string(token.Kind)+":"+string(runes[token.Start:token.End]))
	}
	return out
}

func expectTokens(t *testing.T, source string, expected ...string) {
	t.Helper()
	got := texts(source, Tokenize(source))
	if len(got) != len(expected) {
		t.Fatalf("Expected %d tokens, got %d:\n  %s", len(expected), len(got), strings.Join(got, "\n  "))
	}
	for i, want := range expected {
		if got[i] != want {
			t.Errorf("Expected token %d to be %q, got %q", i, want, got[i])
		}
	}
}

func TestTokenize(t *testing.T) {
	t.Run("Success: a complete block", func(t *testing.T) {
		source := "### Current user\n" +
			"# the token comes from the file variables\n" +
			"GET https://api.github.com/user HTTP/1.1\n" +
			"Accept: application/json\n" +
			"\n" +
			"{ \"note\": 1 }\n"

		expectTokens(t, source,
			"separator:### Current user",
			"comment:# the token comes from the file variables",
			"method:GET",
			"target:https://api.github.com/user",
			"version:HTTP/1.1",
			"header:Accept",
			"value:application/json",
			"body:{ \"note\": 1 }",
		)
	})

	t.Run("Success: variables are painted apart from their values", func(t *testing.T) {
		expectTokens(t, "@baseUrl = https://api.github.com\n",
			"variable:@baseUrl",
			"value:https://api.github.com",
		)
	})

	t.Run("Success: placeholders are lifted out of the token holding them", func(t *testing.T) {
		source := "GET {{baseUrl}}/user/{{id}}\n" +
			"Authorization: Bearer {{token}}\n"

		expectTokens(t, source,
			"method:GET",
			"placeholder:{{baseUrl}}",
			"target:/user/",
			"placeholder:{{id}}",
			"header:Authorization",
			"value:Bearer ",
			"placeholder:{{token}}",
		)
	})

	t.Run("Success: an unclosed placeholder is left as plain text", func(t *testing.T) {
		// Half-typed braces must not swallow the rest of the line.
		expectTokens(t, "@url = https://{{host\n",
			"variable:@url",
			"value:https://{{host",
		)
	})

	t.Run("Success: a block that fails still shows what is readable", func(t *testing.T) {
		// "GE" is not a verb, so the block never parses — and everything around
		// the line being typed must stay painted all the same.
		source := "### Broken\n" +
			"# a note\n" +
			"GE https://example.com/a\n" +
			"Accept: application/json\n"

		expectTokens(t, source,
			"separator:### Broken",
			"comment:# a note",
			"header:Accept",
			"value:application/json",
		)
	})

	t.Run("Success: a JSON body inside a broken block is not painted as headers", func(t *testing.T) {
		source := "### Broken\n" +
			"GE https://example.com/a\n" +
			"\n" +
			"{ \"body\": \"hi\" }\n"

		for _, token := range texts(source, Tokenize(source)) {
			if strings.HasPrefix(token, "header:") {
				t.Errorf("Expected no header token, got %q", token)
			}
		}
	})

	t.Run("Success: a bare separator at the end of a file is painted", func(t *testing.T) {
		// The comment parsers need their line break: the last line of a file
		// that ends without one is only reachable through the recovery path.
		source := "GET https://example.com/a\n\n###"
		got := texts(source, Tokenize(source))
		if len(got) == 0 || got[len(got)-1] != "separator:###" {
			t.Errorf("Expected a trailing separator token, got %v", got)
		}
	})

	t.Run("Success: an empty source yields no token", func(t *testing.T) {
		if tokens := Tokenize(""); len(tokens) != 0 {
			t.Errorf("Expected no token, got %v", tokens)
		}
	})

	t.Run("Success: the tokens never overlap and stay ordered", func(t *testing.T) {
		source := "@baseUrl = https://api.github.com\n\n" +
			"### One\nGET {{baseUrl}}/a\nAccept: */*\n\nbody\n\n" +
			"### Broken\nGE oops\n\n" +
			"### Two\nPOST {{baseUrl}}/b HTTP/1.1\n\n{}\n"

		end := 0
		for _, token := range Tokenize(source) {
			if token.Start < end {
				t.Fatalf("Token %+v starts before the previous one ended at %d", token, end)
			}
			if token.End <= token.Start {
				t.Fatalf("Token %+v is empty", token)
			}
			end = token.End
		}
	})
}

var tagRe = regexp.MustCompile(`</?span[^>]*>`)

// The overlay sits behind the textarea, character for character. Any rune the
// renderer drops, duplicates or moves shows up as text drifting out of its
// highlight, so the round trip is the property that matters most here.
func TestHighlightHTMLRendersTheSourceBackExactly(t *testing.T) {
	sources := map[string]string{
		"empty":            "",
		"blank lines":      "\n\n\n",
		"sample file":      sampleFile,
		"CRLF":             "### Block\r\nGET https://example.com/a HTTP/1.1\r\nAccept: */*\r\n\r\n{}\r\n",
		"broken block":     "### Broken\nGE oops\nAccept: */*\n",
		"no trailing line": "@a = 1\n\n### One\nGET https://example.com",
		"markup in a body": "GET https://example.com/a\n\n<script>alert(\"&\" < 1)</script>\n",
		"unicode":          "# café — ☕️ 42\nGET https://example.com/é\nAccept: */*\n\n{ \"emoji\": \"🙂\" }\n",
		"full file": "@baseUrl = https://api.github.com\n@token = ghp_example\n\n" +
			"### Current user\n# a note\nGET {{baseUrl}}/user HTTP/1.1\n" +
			"Authorization: Bearer {{token}}\n\n{ \"body\": \"hi\" }\n",
	}

	for name, source := range sources {
		t.Run(name, func(t *testing.T) {
			rendered := HighlightHTML(source)
			stripped := tagRe.ReplaceAllString(rendered, "")
			unescaped := strings.NewReplacer("&lt;", "<", "&gt;", ">", "&amp;", "&").Replace(stripped)

			// HighlightHTML adds the line break a <pre> would swallow.
			if unescaped != source+"\n" {
				t.Errorf("The rendered text does not match the source.\n got: %q\nwant: %q", unescaped, source+"\n")
			}
		})
	}
}

func TestHighlightHTMLEscapesMarkup(t *testing.T) {
	rendered := HighlightHTML("GET https://example.com/a\n\n<img src=x onerror=alert(1)>\n")
	if strings.Contains(rendered, "<img") {
		t.Errorf("Expected the body markup to be escaped, got %q", rendered)
	}
	if !strings.Contains(rendered, "&lt;img") {
		t.Errorf("Expected an escaped tag in the output, got %q", rendered)
	}
}
