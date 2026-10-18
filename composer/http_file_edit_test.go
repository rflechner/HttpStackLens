package composer

import (
	"strings"
	"testing"
)

const editSample = `@base = https://api.example.com

### List users
# paged by default
GET {{base}}/users
Accept: application/json
# X-Debug: 1

### Create user
POST {{base}}/users HTTP/1.1
Content-Type: application/json

{"name": "ada"}

###
`

func TestReadRequestsGivesTitlesAndValues(t *testing.T) {
	blocks := ReadRequests(editSample)
	if len(blocks) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(blocks))
	}
	first := blocks[0]
	if first.Title != "List users" || first.Method != "GET" || first.Target != "{{base}}/users" {
		t.Fatalf("unexpected first block: %+v", first)
	}
	if len(first.Headers) != 1 || first.Headers[0] != (HeaderLine{Name: "Accept", Value: "application/json"}) {
		t.Fatalf("unexpected headers: %+v", first.Headers)
	}
	if blocks[1].Body != `{"name": "ada"}` {
		t.Fatalf("unexpected body: %q", blocks[1].Body)
	}
}

func TestAppendRequestReusesADanglingSeparator(t *testing.T) {
	updated, index, err := AppendRequest(editSample, RequestBlock{
		Title:   "Delete user",
		Method:  "delete",
		Target:  "{{base}}/users/1",
		Headers: []HeaderLine{{Name: "Authorization", Value: "Bearer {{token}}"}},
	})
	if err != nil {
		t.Fatalf("AppendRequest: %v", err)
	}
	if index != 2 {
		t.Fatalf("expected index 2, got %d", index)
	}
	if strings.Contains(updated, "###\n\n### Delete user") {
		t.Fatalf("the dangling separator was kept:\n%s", updated)
	}
	blocks := ReadRequests(updated)
	if len(blocks) != 3 || blocks[2].Title != "Delete user" || blocks[2].Method != "DELETE" {
		t.Fatalf("unexpected blocks: %+v", blocks)
	}
	if !strings.HasPrefix(updated, editSample[:strings.Index(editSample, "\n###\n")]) {
		t.Fatalf("the existing requests changed:\n%s", updated)
	}
}

func TestAppendRequestToAnEmptyFile(t *testing.T) {
	updated, index, err := AppendRequest("", RequestBlock{Method: "GET", Target: "https://example.com"})
	if err != nil {
		t.Fatalf("AppendRequest: %v", err)
	}
	if index != 0 || updated != "###\nGET https://example.com\n" {
		t.Fatalf("unexpected result %d %q", index, updated)
	}
}

func TestReplaceRequestKeepsCommentsAndNeighbours(t *testing.T) {
	updated, err := ReplaceRequest(editSample, 0, func(block RequestBlock) (RequestBlock, error) {
		block.Target = "{{base}}/users?page=2"
		block.Headers = append(block.Headers, HeaderLine{Name: "X-Trace", Value: "on"})
		return block, nil
	})
	if err != nil {
		t.Fatalf("ReplaceRequest: %v", err)
	}
	want := `@base = https://api.example.com

### List users
# paged by default
GET {{base}}/users?page=2
Accept: application/json
X-Trace: on
# X-Debug: 1

### Create user
`
	if !strings.HasPrefix(updated, want) {
		t.Fatalf("unexpected rewrite:\n%s", updated)
	}
	if !strings.HasSuffix(updated, "{\"name\": \"ada\"}\n\n###\n") {
		t.Fatalf("the following blocks changed:\n%s", updated)
	}
}

func TestReplaceRequestKeepsTheVersionAndSetsABody(t *testing.T) {
	updated, err := ReplaceRequest(editSample, 1, func(block RequestBlock) (RequestBlock, error) {
		block.Title = "Create admin"
		block.Body = "{\"name\": \"grace\",\r\n \"admin\": true}\n\n"
		return block, nil
	})
	if err != nil {
		t.Fatalf("ReplaceRequest: %v", err)
	}
	if !strings.Contains(updated, "### Create admin\nPOST {{base}}/users HTTP/1.1\n") {
		t.Fatalf("title or version lost:\n%s", updated)
	}
	blocks := ReadRequests(updated)
	if blocks[1].Body != "{\"name\": \"grace\",\n \"admin\": true}" {
		t.Fatalf("unexpected body %q", blocks[1].Body)
	}
}

func TestReplaceRequestRefusesWhatTheFormatCannotHold(t *testing.T) {
	cases := map[string]func(RequestBlock) RequestBlock{
		"method":       func(b RequestBlock) RequestBlock { b.Method = "FETCH"; return b },
		"url space":    func(b RequestBlock) RequestBlock { b.Target = "https://x/a b"; return b },
		"empty url":    func(b RequestBlock) RequestBlock { b.Target = " "; return b },
		"title break":  func(b RequestBlock) RequestBlock { b.Title = "a\nGET https://evil"; return b },
		"header name":  func(b RequestBlock) RequestBlock { b.Headers = []HeaderLine{{Name: "# X", Value: "1"}}; return b },
		"header break": func(b RequestBlock) RequestBlock { b.Headers = []HeaderLine{{Name: "X", Value: "1\nY: 2"}}; return b },
		"body block":   func(b RequestBlock) RequestBlock { b.Body = "a\n### b\nGET https://x"; return b },
	}
	for name, change := range cases {
		_, err := ReplaceRequest(editSample, 0, func(b RequestBlock) (RequestBlock, error) { return change(b), nil })
		if err == nil {
			t.Errorf("%s: expected a refusal", name)
		}
	}
	if _, err := ReplaceRequest(editSample, 5, func(b RequestBlock) (RequestBlock, error) { return b, nil }); err == nil {
		t.Error("expected an out of range index to be refused")
	}
}

func TestReplaceRequestKeepsAnUnreadableBlock(t *testing.T) {
	source := "### One\nGET https://example.com/1\n\n### Broken\nthis is not a request\n\n### Two\nGET https://example.com/2\n"
	updated, err := ReplaceRequest(source, 0, func(b RequestBlock) (RequestBlock, error) {
		b.Target = "https://example.com/one"
		return b, nil
	})
	if err != nil {
		t.Fatalf("ReplaceRequest: %v", err)
	}
	if !strings.Contains(updated, "### Broken\nthis is not a request\n") || !strings.Contains(updated, "example.com/one") {
		t.Fatalf("unexpected rewrite:\n%s", updated)
	}
}

func TestInterpolateReportsUnknownNames(t *testing.T) {
	file := ParseHttpFile("@base = https://api.example.com\n@base = https://ignored\n")
	got, unresolved := Interpolate("{{ base }}/users/{{id}}", file.Variables)
	if got != "https://api.example.com/users/{{id}}" {
		t.Fatalf("unexpected interpolation %q", got)
	}
	if len(unresolved) != 1 || unresolved[0] != "id" {
		t.Fatalf("unexpected unresolved %v", unresolved)
	}
}
