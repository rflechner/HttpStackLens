package mcp

import (
	"context"
	"encoding/json"
	"httpStackLens/composer"
	"httpStackLens/webui/wasm/shared"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const composerSample = `@base = https://api.example.com
@t = s3cr3t-token
@api_key = k3y

### Current user
GET {{base}}/me?page=1&access_token=lit3ral
Authorization: Bearer {{t}}
X-Api-Key: {{api_key}}
Accept: application/json

### Create
POST {{base}}/items
Cookie: session=c00kie

{"name": "x"}
`

func composerSession(t *testing.T, send ComposerSender) (context.Context, *sdk.ClientSession, *composer.HttpFileStore) {
	t.Helper()
	store := composer.NewHttpFileStore(t.TempDir())
	if _, err := store.Save("api.http", composerSample); err != nil {
		t.Fatal(err)
	}
	ctx, session := connectStatusClient(t, Dependencies{HttpFiles: store, SendComposer: send})
	return ctx, session, store
}

// callComposer runs a tool and decodes its structured result, failing the test
// on a tool error unless wantError is set.
func callComposer[T any](t *testing.T, ctx context.Context, session *sdk.ClientSession, name string, args map[string]any, wantError bool) (T, string) {
	t.Helper()
	var out T
	result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	all, _ := json.Marshal(result)
	if result.IsError != wantError {
		t.Fatalf("%s: IsError=%v, want %v: %s", name, result.IsError, wantError, all)
	}
	if !wantError {
		data, _ := json.Marshal(result.StructuredContent)
		if err := json.Unmarshal(data, &out); err != nil {
			t.Fatal(err)
		}
	}
	return out, string(all)
}

func TestComposerToolsAreAdvertised(t *testing.T) {
	ctx, session, _ := composerSession(t, nil)
	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	readOnly := map[string]bool{}
	for _, tool := range listed.Tools {
		readOnly[tool.Name] = tool.Annotations != nil && tool.Annotations.ReadOnlyHint
	}
	for name, want := range map[string]bool{"list_composer_requests": true, "get_composer_request": true, "add_composer_request": false, "update_composer_request": false, "send_composer_request": false} {
		got, found := readOnly[name]
		if !found || got != want {
			t.Errorf("%s: advertised=%v readOnly=%v", name, found, got)
		}
	}
}

func TestListAndGetComposerRequestsRedactCredentials(t *testing.T) {
	ctx, session, _ := composerSession(t, nil)

	list, all := callComposer[ListComposerResult](t, ctx, session, "list_composer_requests", map[string]any{}, false)
	if len(list.Files) != 1 || len(list.Files[0].Requests) != 2 {
		t.Fatalf("unexpected list: %+v", list)
	}
	first := list.Files[0].Requests[0]
	if first.Title != "Current user" || first.Method != "GET" || first.URL != "{{base}}/me?page=1&access_token=__REDACTED__" {
		t.Fatalf("unexpected summary: %+v", first)
	}
	if strings.Contains(all, "lit3ral") {
		t.Fatalf("list leaked a query secret: %s", all)
	}

	detail, all := callComposer[ComposerRequestDetail](t, ctx, session, "get_composer_request", map[string]any{"file": "api", "index": 0}, false)
	for _, secret := range []string{"s3cr3t-token", "k3y", "lit3ral", "c00kie"} {
		if strings.Contains(all, secret) {
			t.Fatalf("get leaked %q: %s", secret, all)
		}
	}
	if detail.Headers[0] != (ComposerHeader{Name: "Authorization", Value: "Bearer {{t}}"}) {
		t.Fatalf("a template header should stay readable: %+v", detail.Headers)
	}
	vars := map[string]string{}
	for _, v := range detail.Variables {
		vars[v.Name] = v.Value
	}
	if vars["base"] != "https://api.example.com" || vars["t"] != redactedValue || vars["api_key"] != redactedValue {
		t.Fatalf("unexpected variables: %+v", vars)
	}

	cookie, _ := callComposer[ComposerRequestDetail](t, ctx, session, "get_composer_request", map[string]any{"file": "api.http", "index": 1}, false)
	if cookie.Headers[0].Value != redactedValue || cookie.Body != `{"name": "x"}` {
		t.Fatalf("unexpected second request: %+v", cookie)
	}

	_, _ = callComposer[ComposerRequestDetail](t, ctx, session, "get_composer_request", map[string]any{"file": "api", "index": 7}, true)
	_, _ = callComposer[ComposerRequestDetail](t, ctx, session, "get_composer_request", map[string]any{"file": "../api", "index": 0}, true)
}

func TestAddAndUpdateComposerRequests(t *testing.T) {
	ctx, session, store := composerSession(t, nil)

	added, _ := callComposer[ComposerWriteResult](t, ctx, session, "add_composer_request", map[string]any{
		"file": "new", "title": "Health", "method": "get", "url": "https://example.com/health",
		"headers": []map[string]any{{"name": "Accept", "value": "text/plain"}},
	}, false)
	if added.File != "new.http" || added.Index != 0 || added.ModifiedAt == "" {
		t.Fatalf("unexpected add result: %+v", added)
	}
	created, err := os.ReadFile(filepath.Join(store.Folder(), "new.http"))
	if err != nil || string(created) != "### Health\nGET https://example.com/health\nAccept: text/plain\n" {
		t.Fatalf("unexpected new file %q: %v", created, err)
	}

	_, _ = callComposer[ComposerWriteResult](t, ctx, session, "add_composer_request", map[string]any{
		"file": "new", "method": "GET", "url": "https://example.com/a b",
	}, true)

	// Hidden values sent back as __REDACTED__ keep what the file holds.
	before, _ := store.Read("api.http")
	updated, _ := callComposer[ComposerWriteResult](t, ctx, session, "update_composer_request", map[string]any{
		"file": "api", "index": 1, "expected_modified_at": before.ModifiedAt,
		"title":   "Create item",
		"headers": []map[string]any{{"name": "Cookie", "value": redactedValue}, {"name": "Content-Type", "value": "application/json"}},
	}, false)
	if updated.Index != 1 {
		t.Fatalf("unexpected update result: %+v", updated)
	}
	after, _ := store.Read("api.http")
	if !strings.Contains(after.Content, "### Create item\nPOST {{base}}/items\nCookie: session=c00kie\nContent-Type: application/json\n\n{\"name\": \"x\"}\n") {
		t.Fatalf("unexpected rewrite:\n%s", after.Content)
	}
	if !strings.HasPrefix(after.Content, composerSample[:strings.Index(composerSample, "### Create")]) {
		t.Fatalf("the first request changed:\n%s", after.Content)
	}

	_, _ = callComposer[ComposerWriteResult](t, ctx, session, "update_composer_request", map[string]any{
		"file": "api", "index": 0, "url": "https://example.com/?access_token=" + redactedValue,
	}, false)
	reread, _ := store.Read("api.http")
	if !strings.Contains(reread.Content, "GET https://example.com/?access_token=lit3ral\n") {
		t.Fatalf("the hidden query value was not kept:\n%s", reread.Content)
	}

	// A stale read is refused rather than overwriting someone else's change.
	_, all := callComposer[ComposerWriteResult](t, ctx, session, "update_composer_request", map[string]any{
		"file": "api", "index": 0, "expected_modified_at": before.ModifiedAt, "body": "x",
	}, true)
	if !strings.Contains(all, "changed since") {
		t.Fatalf("unexpected error: %s", all)
	}
	_, _ = callComposer[ComposerWriteResult](t, ctx, session, "update_composer_request", map[string]any{"file": "missing", "index": 0, "title": "x"}, true)
}

func TestSendComposerRequestResolvesVariables(t *testing.T) {
	var sent []shared.ComposerRequestDto
	send := func(_ context.Context, request shared.ComposerRequestDto) (shared.ComposerResponseDto, error) {
		sent = append(sent, request)
		if strings.Contains(request.Url, "/items") {
			return shared.ComposerResponseDto{Error: "connection refused", DurationMs: 3, Upstream: "http://user:pw@proxy"}, nil
		}
		return shared.ComposerResponseDto{
			Status: 200, StatusText: "OK", Proto: "HTTP/1.1", DurationMs: 12, Upstream: "http://user:pw@proxy",
			Headers: []shared.HeaderDto{{Name: "Set-Cookie", Value: "sid=server-secret"}, {Name: "Content-Type", Value: "application/json"}},
			Body:    `{"user":"ada","padding":"` + strings.Repeat("é", 50) + `"}`,
		}, nil
	}
	ctx, session, store := composerSession(t, send)

	result, all := callComposer[ComposerSendResult](t, ctx, session, "send_composer_request", map[string]any{"file": "api", "index": 0, "max_body_chars": 20}, false)
	if len(sent) != 1 {
		t.Fatalf("expected one send, got %d", len(sent))
	}
	request := sent[0]
	if request.Method != "GET" || request.Url != "https://api.example.com/me?page=1&access_token=lit3ral" || request.Body != "" {
		t.Fatalf("unexpected request: %+v", request)
	}
	if request.Headers[0] != (shared.HeaderDto{Name: "Authorization", Value: "Bearer s3cr3t-token"}) {
		t.Fatalf("variables not substituted in headers: %+v", request.Headers)
	}
	if result.Outcome != "response" || result.Status != 200 || !result.BodyTruncated || result.Body != `{"user":"ada","paddi` {
		t.Fatalf("unexpected result: %+v", result)
	}
	for _, secret := range []string{"server-secret", "s3cr3t-token", "lit3ral", "user:pw"} {
		if strings.Contains(all, secret) {
			t.Fatalf("send leaked %q: %s", secret, all)
		}
	}

	failed, _ := callComposer[ComposerSendResult](t, ctx, session, "send_composer_request", map[string]any{"file": "api", "index": 1}, false)
	if failed.Outcome != "transport_error" || failed.Error != "connection refused" || sent[1].Body != `{"name": "x"}` {
		t.Fatalf("unexpected transport error result: %+v", failed)
	}

	if _, err := store.Save("vars.http", "### Missing\nGET https://{{host}}/x\nX-Id: {{id}}\n"); err != nil {
		t.Fatal(err)
	}
	_, all = callComposer[ComposerSendResult](t, ctx, session, "send_composer_request", map[string]any{"file": "vars", "index": 0}, true)
	if !strings.Contains(all, "host, id") || len(sent) != 2 {
		t.Fatalf("an unresolved request must not be sent: %s", all)
	}
}

func TestSendComposerRequestUnavailable(t *testing.T) {
	ctx, session, _ := composerSession(t, nil)
	_, _ = callComposer[ComposerSendResult](t, ctx, session, "send_composer_request", map[string]any{"file": "api", "index": 0}, true)
}
