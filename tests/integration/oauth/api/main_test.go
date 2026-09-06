package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProtected(t *testing.T) {
	for _, test := range []struct {
		name           string
		header         string
		path           string
		change         func(*tokenInfo)
		upstreamStatus int
		want           int
	}{
		{name: "missing", path: "/protected", want: 401},
		{name: "wrong scheme", header: "Basic abc", path: "/protected", want: 401},
		{name: "valid", header: "Bearer demo", path: "/protected", want: 200},
		{name: "case insensitive bearer", header: "bearer demo", path: "/protected", want: 200},
		{name: "inactive", header: "Bearer demo", path: "/protected", change: func(i *tokenInfo) { i.Active = false }, want: 401},
		{name: "expired", header: "Bearer demo", path: "/protected", change: func(i *tokenInfo) { i.Expires = time.Now().Unix() - 1 }, want: 401},
		{name: "missing expiry", header: "Bearer demo", path: "/protected", change: func(i *tokenInfo) { i.Expires = 0 }, want: 401},
		{name: "wrong issuer", header: "Bearer demo", path: "/protected", change: func(i *tokenInfo) { i.Issuer = "wrong" }, want: 401},
		{name: "wrong audience", header: "Bearer demo", path: "/protected", change: func(i *tokenInfo) { i.Audience = json.RawMessage(`"other"`) }, want: 401},
		{name: "audience array", header: "Bearer demo", path: "/protected", change: func(i *tokenInfo) { i.Audience = json.RawMessage(`["other","demo-api"]`) }, want: 200},
		{name: "scope missing", header: "Bearer demo", path: "/scoped", want: 403},
		{name: "scope exact match", header: "Bearer demo", path: "/scoped", change: func(i *tokenInfo) { i.Scope = "demo:reader" }, want: 403},
		{name: "scope granted", header: "Bearer demo", path: "/scoped", change: func(i *tokenInfo) { i.Scope = "openid demo:read" }, want: 200},
		{name: "provider unavailable", header: "Bearer demo", path: "/protected", upstreamStatus: 500, want: 503},
	} {
		t.Run(test.name, func(t *testing.T) {
			info := tokenInfo{Active: true, Issuer: "http://issuer", Audience: json.RawMessage(`"demo-api"`), Expires: time.Now().Unix() + 60, Subject: "alice"}
			if test.change != nil {
				test.change(&info)
			}
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				user, password, ok := r.BasicAuth()
				if !ok || user != "demo-api" || password != "demo-api-secret" {
					t.Error("missing introspection credentials")
				}
				if r.Method != "POST" || r.FormValue("token") != "demo" {
					t.Error("incorrect introspection request")
				}
				if test.upstreamStatus != 0 {
					w.WriteHeader(test.upstreamStatus)
					return
				}
				_ = json.NewEncoder(w).Encode(info)
			}))
			defer provider.Close()
			a := &api{issuer: "http://issuer", introspectionURL: provider.URL, client: provider.Client()}
			request := httptest.NewRequest("GET", test.path, nil)
			request.Header.Set("Authorization", test.header)
			response := httptest.NewRecorder()
			a.routes().ServeHTTP(response, request)
			if response.Code != test.want {
				t.Fatalf("got %d: %s, want %d", response.Code, response.Body.String(), test.want)
			}
			if (test.want == 401 || test.want == 403) && response.Header().Get("WWW-Authenticate") == "" {
				t.Error("missing authentication challenge")
			}
		})
	}
}

func TestMalformedIntrospection(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("not JSON")) }))
	defer provider.Close()
	a := &api{issuer: "http://issuer", introspectionURL: provider.URL, client: provider.Client()}
	request := httptest.NewRequest("GET", "/protected", nil)
	request.Header.Set("Authorization", "Bearer demo")
	response := httptest.NewRecorder()
	a.routes().ServeHTTP(response, request)
	if response.Code != 503 {
		t.Fatalf("got %d, want 503", response.Code)
	}
}
