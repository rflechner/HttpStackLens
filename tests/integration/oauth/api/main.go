package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

//go:embed index.html
var page string

type tokenInfo struct {
	Active   bool            `json:"active"`
	Issuer   string          `json:"iss"`
	Audience json.RawMessage `json:"aud"`
	Expires  int64           `json:"exp"`
	Subject  string          `json:"sub"`
	Scope    string          `json:"scope"`
}

type api struct {
	issuer           string
	introspectionURL string
	client           *http.Client
}

func main() {
	a := &api{
		issuer:           os.Getenv("OAUTH_ISSUER"),
		introspectionURL: os.Getenv("OAUTH_INTROSPECTION_URL"),
		client:           &http.Client{Timeout: 4 * time.Second},
	}
	if len(os.Args) == 2 && os.Args[1] == "-healthcheck" {
		response, err := a.client.Get("http://localhost:8080/ready")
		if err != nil {
			os.Exit(1)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		return
	}
	if a.issuer == "" || a.introspectionURL == "" {
		log.Fatal("OAuth environment variables are required")
	}
	server := &http.Server{Addr: ":8080", Handler: a.routes(), ReadHeaderTimeout: 5 * time.Second}
	log.Print("OAuth demo listening on :8080")
	log.Fatal(server.ListenAndServe())
}

func (a *api) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = io.WriteString(w, page)
	})
	mux.HandleFunc("GET /public", func(w http.ResponseWriter, r *http.Request) {
		reply(w, 200, map[string]string{"message": "Hello from the demo API"})
	})
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, r *http.Request) {
		// Even an inactive token proves the realm and confidential client are ready.
		if _, err := a.inspect(r, "readiness-probe"); err != nil {
			reply(w, 503, map[string]string{"error": "keycloak_unavailable"})
			return
		}
		reply(w, 200, map[string]string{"status": "ready"})
	})
	mux.HandleFunc("GET /protected", a.protected(""))
	mux.HandleFunc("GET /scoped", a.protected("demo:read"))
	return mux
}

func (a *api) inspect(r *http.Request, token string) (tokenInfo, error) {
	var info tokenInfo
	form := url.Values{"token": {token}, "token_type_hint": {"access_token"}}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, a.introspectionURL, strings.NewReader(form.Encode()))
	if err != nil {
		return info, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.SetBasicAuth("demo-api", "demo-api-secret")
	response, err := a.client.Do(request)
	if err != nil {
		return info, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return info, fmt.Errorf("introspection status: %d", response.StatusCode)
	}
	err = json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&info)
	return info, err
}

func audienceContains(raw json.RawMessage, expected string) bool {
	var single string
	if json.Unmarshal(raw, &single) == nil {
		return single == expected
	}
	var multiple []string
	if json.Unmarshal(raw, &multiple) != nil {
		return false
	}
	for _, value := range multiple {
		if value == expected {
			return true
		}
	}
	return false
}

func (a *api) protected(scope string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Fields(r.Header.Get("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			w.Header().Set("WWW-Authenticate", `Bearer realm="demo-api"`)
			reply(w, 401, map[string]string{"error": "missing_bearer_token"})
			return
		}
		info, err := a.inspect(r, parts[1])
		if err != nil {
			reply(w, 503, map[string]string{"error": "keycloak_unavailable"})
			return
		}
		if !info.Active || info.Issuer != a.issuer || !audienceContains(info.Audience, "demo-api") || info.Expires <= time.Now().Unix() {
			w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
			reply(w, 401, map[string]string{"error": "invalid_token"})
			return
		}
		allowed := scope == ""
		for _, value := range strings.Fields(info.Scope) {
			if value == scope {
				allowed = true
			}
		}
		if !allowed {
			w.Header().Set("WWW-Authenticate", `Bearer error="insufficient_scope", scope="demo:read"`)
			reply(w, 403, map[string]string{"error": "insufficient_scope"})
			return
		}
		reply(w, 200, map[string]string{"message": "Access granted", "subject": info.Subject, "scope": info.Scope})
	}
}

func reply(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
