package composer

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	p "github.com/rflechner/EasyParsingForGo/combinator"
)

func TestFunctionCallParser(t *testing.T) {
	for _, test := range []struct {
		source string
		want   map[string]string
	}{
		{`oauth(clientId:"xxxxx", clientSecret: popopopo)`, map[string]string{"clientId": "xxxxx", "clientSecret": "popopopo"}},
		{"oauth(\r\n clientId: \"démo 🚀\",\r\n scope: \"demo:read other\",\r\n)", map[string]string{"clientId": "démo 🚀", "scope": "demo:read other"}},
		{`oauth(secret: "a,):\"b\\c\n", empty: "")`, map[string]string{"secret": "a,):\"b\\c\n", "empty": ""}},
		{`oauth(url: https://localhost:18080/token, enabled: true, count: 12)`, map[string]string{"url": "https://localhost:18080/token", "enabled": "true", "count": "12"}},
		{`oauth ()`, map[string]string{}},
	} {
		result, err := FunctionCallParser()(p.NewParsingContext(test.source + "\nnext"))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if result.Result.Name != "oauth" || !reflect.DeepEqual(result.Result.Parameters, test.want) {
			t.Fatalf("unexpected call: %#v", result.Result)
		}
		if string(result.Context.Remaining) != "\nnext" {
			t.Fatalf("consumed beyond call: %q", string(result.Context.Remaining))
		}
	}
}

func TestFunctionCallParserRejectsInvalidSyntax(t *testing.T) {
	for _, source := range []string{
		`oauth`, `oauth(`, `oauth("positional")`, `oauth(key value)`,
		`oauth(key:)`, `oauth(key: a key2: b)`, `oauth(key:a,key:b)`,
		`oauth(key: "unterminated)`, `oauth(key: "\q")`,
		`oauth(key: env(name:"SECRET"))`, `oauth(key:a,,)`,
		"oauth(key: \"line\nbreak\")", `oauth(key: 'single quotes')`,
	} {
		if _, err := FunctionCallParser()(p.NewParsingContext(source)); err == nil {
			t.Errorf("accepted invalid syntax: %q", source)
		}
	}
}

func TestFunctionVariableFileAndSpans(t *testing.T) {
	source := "@base = https://localhost/path(x)\r\n@token = oauth(\r\n clientId: \"démo\",\r\n clientSecret: secret,\r\n)  \r\n### Protected\r\nGET {{base}}\r\nAuthorization: Bearer {{token}}\r\n"
	file := ParseHttpFile(source)
	if len(file.Issues) != 0 || len(file.Variables) != 2 || len(file.Requests) != 1 {
		t.Fatalf("unexpected file: %+v", file)
	}
	if file.Variables[0].Call != nil {
		t.Fatal("literal URL interpreted as call")
	}
	v := file.Variables[1]
	if v.Call == nil || v.Call.Parameters["clientId"] != "démo" {
		t.Fatalf("missing call: %+v", v)
	}
	if got := slice(t, source, v.Value.Start, v.Value.End); got != v.Value.Text {
		t.Fatalf("value span differs from source: %q", got)
	}
	if file.Requests[0].Headers[0].Value.Text != "Bearer {{token}}" {
		t.Fatal("request after multiline call was changed")
	}
	// Highlighting must preserve the source and keep all offsets in bounds.
	for _, token := range Tokenize(source) {
		if token.Start < 0 || token.End > len([]rune(source)) {
			t.Fatalf("invalid token span: %+v", token)
		}
	}
	if !strings.Contains(HighlightHTML(source), "clientId:") {
		t.Fatal("multiline call disappeared from highlighting")
	}
}

func TestFunctionVariableErrorsRecoverAtNextRequest(t *testing.T) {
	for _, declaration := range []string{
		"@token = oauth(key: private-secret, key: other)",
		"@token = oauth(\nkey: private-secret\n",
		"@token = oauth(key: private-secret) trailing",
	} {
		file := ParseHttpFile(declaration + "\n### Next\nGET https://example.com\n")
		if len(file.Issues) != 1 || len(file.Variables) != 0 || len(file.Requests) != 1 {
			t.Fatalf("failed recovery: %+v", file)
		}
		if strings.Contains(file.Issues[0].Message, "private-secret") {
			t.Fatal("diagnostic exposed a parameter value")
		}
	}
}

func TestFileVariableEvaluate(t *testing.T) {
	file := ParseHttpFile("@literal = value\n@token = oauth(clientId: demo, clientSecret: secret)\n")
	if len(file.Issues) != 0 || len(file.Variables) != 2 {
		t.Fatalf("parse: %+v", file)
	}
	ctx := context.Background()
	value, err := file.Variables[0].Evaluate(ctx, nil)
	if err != nil || value != "value" {
		t.Fatalf("literal: %q, %v", value, err)
	}
	calls := 0
	handler := func(gotContext context.Context, name string, parameters map[string]string) (string, error) {
		calls++
		if gotContext != ctx || name != "oauth" || parameters["clientId"] != "demo" || parameters["clientSecret"] != "secret" {
			t.Fatal("incorrect handler arguments")
		}
		parameters["clientSecret"] = "changed"
		return "access-token", nil
	}
	value, err = file.Variables[1].Evaluate(ctx, handler)
	if err != nil || value != "access-token" || calls != 1 {
		t.Fatalf("dispatch: %q, %v, calls=%d", value, err, calls)
	}
	if file.Variables[1].Call.Parameters["clientSecret"] != "secret" {
		t.Fatal("handler mutated parsed parameters")
	}
	if _, err := file.Variables[1].Evaluate(ctx, nil); err == nil {
		t.Fatal("missing handler should fail")
	}
	wantError := errors.New("unsupported function")
	_, err = file.Variables[1].Evaluate(ctx, func(context.Context, string, map[string]string) (string, error) { return "", wantError })
	if !errors.Is(err, wantError) {
		t.Fatalf("handler error lost: %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := file.Variables[1].Evaluate(cancelled, handler); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
	if calls != 1 {
		t.Fatal("cancelled evaluation invoked handler")
	}
}
