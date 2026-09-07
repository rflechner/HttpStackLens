package webui

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func oauthRequest(s *composerOAuth, method, path, body, id string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://localhost:9000"+path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "http://localhost:9000")
	r.Header.Set("X-OAuth-ID", id)
	w := httptest.NewRecorder()
	switch path {
	case "/api/composer/oauth":
		s.start(w, r)
	case "/api/composer/oauth/result":
		s.result(w, r)
	default:
		s.callback(w, r)
	}
	return w
}

func TestComposerOAuthPKCE(t *testing.T) {
	var issuer, challenge string
	exchanges := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "openid-configuration") {
			_ = json.NewEncoder(w).Encode(map[string]string{"issuer": issuer, "authorization_endpoint": issuer + "/auth", "token_endpoint": issuer + "/token"})
			return
		}
		exchanges++
		_ = r.ParseForm()
		hash := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		if base64.RawURLEncoding.EncodeToString(hash[:]) != challenge || r.Form.Get("redirect_uri") != "http://localhost:9000/api/composer/oauth/callback" || r.Form.Get("code") != "test-code" || r.Form.Get("grant_type") != "authorization_code" {
			t.Error("incorrect PKCE exchange")
		}
		_, _ = w.Write([]byte(`{"access_token":"private-token","token_type":"Bearer","expires_in":120}`))
	}))
	defer provider.Close()
	issuer = provider.URL
	s := newComposerOAuth()
	w := oauthRequest(s, "POST", "/api/composer/oauth", `{"issuer":"`+issuer+`","clientId":"demo","scope":"demo:read","prompt":"login"}`, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var start oauthAnswer
	_ = json.Unmarshal(w.Body.Bytes(), &start)
	auth, err := url.Parse(start.AuthorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	query := auth.Query()
	challenge = query.Get("code_challenge")
	if query.Get("code_challenge_method") != "S256" || query.Get("state") == "" || query.Get("state") == start.ID || query.Get("scope") != "demo:read" || query.Get("prompt") != "login" {
		t.Fatal("invalid authorization parameters")
	}
	if w := oauthRequest(s, "POST", "/api/composer/oauth/result", "", start.ID); w.Code != 202 {
		t.Fatal(w.Code)
	}
	if w := oauthRequest(s, "POST", "/api/composer/oauth/result", "", query.Get("state")); w.Code != 404 {
		t.Fatal("state can retrieve tokens")
	}
	invalid := oauthRequest(s, "GET", oauthCallbackPath+"?state=invalid&code=test-code", "", "")
	if invalid.Code != 400 || exchanges != 0 {
		t.Fatal("invalid state accepted")
	}
	callback := oauthCallbackPath + "?state=" + url.QueryEscape(query.Get("state")) + "&code=test-code"
	if w := oauthRequest(s, "GET", callback, "", ""); w.Code != 200 || strings.Contains(w.Body.String(), "private-token") {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := oauthRequest(s, "GET", callback, "", ""); w.Code != 400 {
		t.Fatal("callback replay accepted")
	}
	w = oauthRequest(s, "POST", "/api/composer/oauth/result", "", start.ID)
	var token oauthAnswer
	_ = json.Unmarshal(w.Body.Bytes(), &token)
	if w.Code != 200 || token.AccessToken != "private-token" || token.ExpiresIn != 120 || exchanges != 1 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Code, exchanges)
	}
	if w := oauthRequest(s, "POST", "/api/composer/oauth/result", "", start.ID); w.Code != 404 {
		t.Fatal("result replay accepted")
	}
}

func TestComposerOAuthClientCredentials(t *testing.T) {
	var issuer string
	fail := false
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "openid-configuration") {
			_ = json.NewEncoder(w).Encode(map[string]string{"issuer": issuer, "token_endpoint": issuer + "/token"})
			return
		}
		_ = r.ParseForm()
		if _, exists := r.Form["scope"]; exists {
			t.Error("empty scope must be omitted")
		}
		if r.Form.Get("client_secret") != "private-secret" || r.Form.Get("grant_type") != "client_credentials" {
			t.Error("wrong client credentials")
		}
		if fail {
			http.Error(w, "private-secret provider debug", 401)
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"service-token","token_type":"bearer"}`))
	}))
	defer provider.Close()
	issuer = provider.URL
	s := newComposerOAuth()
	body := `{"issuer":"` + issuer + `","clientId":"service","clientSecret":"private-secret","grantType":"client_credentials"}`
	w := oauthRequest(s, "POST", "/api/composer/oauth", body, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "service-token") || len(s.attempts) != 0 {
		t.Fatal(w.Code)
	}
	fail = true
	w = oauthRequest(s, "POST", "/api/composer/oauth", body, "")
	if w.Code != 502 || strings.Contains(w.Body.String(), "private-secret") {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestComposerOAuthCancelExpiryAndOrigin(t *testing.T) {
	s := newComposerOAuth()
	s.attempts["cancel"] = &oauthAttempt{expires: time.Now().Add(time.Minute)}
	if w := oauthRequest(s, "DELETE", "/api/composer/oauth/result", "", "cancel"); w.Code != 204 {
		t.Fatal(w.Code)
	}
	s.attempts["expired"] = &oauthAttempt{expires: time.Now().Add(-time.Minute)}
	if w := oauthRequest(s, "POST", "/api/composer/oauth/result", "", "expired"); w.Code != 404 {
		t.Fatal(w.Code)
	}
	r := httptest.NewRequest("POST", "http://localhost:9000/api/composer/oauth", strings.NewReader(`{}`))
	r.Header.Set("Origin", "https://untrusted.example")
	w := httptest.NewRecorder()
	s.start(w, r)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
	for _, raw := range []string{"http://example.com/realm", "https://user:secret@example.com", "javascript:alert(1)", "https://example.com/#fragment"} {
		if _, err := oauthURL(raw); err == nil {
			t.Fatal(raw)
		}
	}
	for _, raw := range []string{"http://localhost:18080/realms/demo", "http://127.0.0.1:18080/realms/demo", "https://login.example.com/realm"} {
		if _, err := oauthURL(raw); err != nil {
			t.Fatal(raw, err)
		}
	}
	s.attempts["denied"] = &oauthAttempt{expires: time.Now().Add(time.Minute), state: "denied-state", redirectURI: "http://localhost:9000" + oauthCallbackPath}
	if w := oauthRequest(s, "GET", oauthCallbackPath+"?state=denied-state&error=access_denied", "", ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
	w = oauthRequest(s, "POST", "/api/composer/oauth/result", "", "denied")
	if !strings.Contains(w.Body.String(), "denied") {
		t.Fatal(w.Body.String())
	}
}

// Opt in against tests/integration/oauth/compose.yaml. No UI server is started.
func TestComposerOAuthKeycloak(t *testing.T) {
	if os.Getenv("HSL_TEST_KEYCLOAK") != "1" {
		t.Skip("set HSL_TEST_KEYCLOAK=1 with the local OAuth fixture running")
	}
	s := newComposerOAuth()
	for _, scope := range []string{"demo:read", ""} {
		payload, _ := json.Marshal(oauthOptions{Issuer: "http://localhost:18080/realms/httpstacklens", ClientID: "composer-service", ClientSecret: "composer-demo-secret", GrantType: "client_credentials", Scope: scope})
		w := oauthRequest(s, "POST", "/api/composer/oauth", string(payload), "")
		if w.Code != 200 {
			t.Fatalf("token endpoint status %d: %s", w.Code, w.Body.String())
		}
		var answer oauthAnswer
		if json.Unmarshal(w.Body.Bytes(), &answer) != nil || answer.AccessToken == "" {
			t.Fatal("missing token")
		}
		req, _ := http.NewRequest("GET", "http://localhost:18081/scoped", nil)
		req.Header.Set("Authorization", "Bearer "+answer.AccessToken)
		res, err := s.client.Do(req)
		if err != nil {
			t.Fatal("local API unavailable")
		}
		res.Body.Close()
		want := http.StatusOK
		if scope == "" {
			want = http.StatusForbidden
		}
		if res.StatusCode != want {
			t.Fatalf("API status %d, want %d", res.StatusCode, want)
		}
	}
}

func TestComposerOAuthOpenBrowser(t *testing.T) {
	s := newComposerOAuth()
	const target = "https://identity.example/auth?state=fixture"
	s.attempts["handle"] = &oauthAttempt{authorizationURL: target, expires: time.Now().Add(time.Minute)}
	calls := 0
	s.openBrowser = func(got string) error {
		calls++
		if got != target {
			t.Fatal("did not use stored authorization URL")
		}
		return nil
	}
	invoke := func(method, id, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://localhost:9000/api/composer/oauth/open?url=file:///ignored", nil)
		r.Header.Set("X-OAuth-ID", id)
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		s.open(w, r)
		return w
	}
	if w := invoke("POST", "handle", "https://other.example"); w.Code != 403 {
		t.Fatal(w.Code)
	}
	if w := invoke("GET", "handle", ""); w.Code != 405 {
		t.Fatal(w.Code)
	}
	if w := invoke("POST", "unknown", ""); w.Code != 404 {
		t.Fatal(w.Code)
	}
	if calls != 0 {
		t.Fatal("invalid request opened browser")
	}
	for i := 0; i < 2; i++ {
		if w := invoke("POST", "handle", "http://localhost:9000"); w.Code != 204 {
			t.Fatal(w.Code)
		}
	}
	if calls != 1 {
		t.Fatal("duplicate browser launch")
	}
	s.attempts["handle"].answer = &oauthAnswer{}
	if w := invoke("POST", "handle", ""); w.Code != 404 {
		t.Fatal(w.Code)
	}
}
