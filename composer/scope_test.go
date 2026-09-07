package composer

import (
	"context"
	"errors"
	"testing"
)

func TestScopeLifecycle(t *testing.T) {
	file := ParseHttpFile("@issuer = https://login.example\n@token = oauth(issuer: '{{issuer}}', clientId: 'demo')\n@auth = Bearer {{token}}\n\n### test\nGET https://example.com\n")
	if len(file.Issues) > 0 {
		t.Fatal(file.Issues)
	}
	var scope Scope
	calls := 0
	handler := func(_ context.Context, name string, params map[string]string) (string, error) {
		calls++
		if name != "oauth" || params["issuer"] != "https://login.example" {
			t.Fatal(name, params)
		}
		return "secret", nil
	}
	for i := 0; i < 2; i++ {
		values, err := scope.Evaluate(context.Background(), file.Variables, handler)
		if err != nil || values["auth"] != "Bearer secret" {
			t.Fatal(values, err)
		}
		values["token"] = "tampered"
	}
	if calls != 1 {
		t.Fatal(calls)
	}
	if file.Variables[1].Call.Parameters["issuer"] != "{{issuer}}" {
		t.Fatal("mutated AST")
	}
	scope.Reset()
	if _, err := scope.Evaluate(context.Background(), file.Variables, handler); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatal(calls)
	}
	file.Variables[0].Value.Text = "https://login.example/changed"
	if scope.Ready(file.Variables) {
		t.Fatal("edited declarations reused old scope")
	}
}

func TestScopeFailureAndIsolation(t *testing.T) {
	vars := ParseHttpFile("@token = oauth(clientId: demo)\n").Variables
	var first, second Scope
	failure := errors.New("login cancelled")
	if _, err := first.Evaluate(context.Background(), vars, func(context.Context, string, map[string]string) (string, error) { return "", failure }); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if first.Ready(vars) {
		t.Fatal("cached failure")
	}
	if _, err := first.Evaluate(context.Background(), vars, func(context.Context, string, map[string]string) (string, error) { return "ok", nil }); err != nil {
		t.Fatal(err)
	}
	if second.Ready(vars) {
		t.Fatal("shared scope between files")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := first.Evaluate(ctx, vars, nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestScopeRejectsInvalidReferences(t *testing.T) {
	for _, source := range []string{"@a = {{b}}\n@b = value\n", "@a = value\n@a = other\n"} {
		var scope Scope
		if _, err := scope.Evaluate(context.Background(), ParseHttpFile(source).Variables, nil); err == nil {
			t.Fatal(source)
		}
	}
}
