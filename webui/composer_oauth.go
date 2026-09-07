package webui

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/pkg/browser"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const oauthCallbackPath = "/api/composer/oauth/callback"
const oauthLifetime = 5 * time.Minute

type oauthOptions struct {
	Issuer       string `json:"issuer"`
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret,omitempty"`
	GrantType    string `json:"grantType,omitempty"`
	Scope        string `json:"scope,omitempty"`
	Prompt       string `json:"prompt,omitempty"`
}

type oauthAnswer struct {
	ID               string `json:"id,omitempty"`
	AuthorizationURL string `json:"authorizationUrl,omitempty"`
	AccessToken      string `json:"accessToken,omitempty"`
	ExpiresIn        int    `json:"expiresIn,omitempty"`
	Error            string `json:"error,omitempty"`
}

type oauthAttempt struct {
	options                                oauthOptions
	tokenURL, redirectURI, verifier, state string
	authorizationURL                       string
	opened                                 bool
	expires                                time.Time
	processing                             bool
	answer                                 *oauthAnswer
}

type composerOAuth struct {
	openBrowser func(string) error
	client      *http.Client
	mu          sync.Mutex
	attempts    map[string]*oauthAttempt
}

func newComposerOAuth() *composerOAuth {
	return &composerOAuth{
		openBrowser: browser.OpenURL,
		client:      &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		attempts:    map[string]*oauthAttempt{},
	}
}

func (s *composerOAuth) register(mux *http.ServeMux) {
	mux.HandleFunc("/api/composer/oauth", s.start)
	mux.HandleFunc("/api/composer/oauth/result", s.result)
	mux.HandleFunc("/api/composer/oauth/open", s.open)
	mux.HandleFunc(oauthCallbackPath, s.callback)
}

func oauthURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.RawQuery != "" {
		return nil, errors.New("invalid OAuth URL")
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || ip != nil && ip.IsLoopback())) {
		return nil, errors.New("OAuth requires HTTPS (HTTP is allowed on loopback only)")
	}
	return u, nil
}

func oauthOrigin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func oauthLocalRequest(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Cache-Control", "no-store")
	if origin := r.Header.Get("Origin"); origin != "" && origin != oauthOrigin(r) {
		http.Error(w, "cross-origin OAuth request refused", http.StatusForbidden)
		return false
	}
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
		http.Error(w, "cross-origin OAuth request refused", http.StatusForbidden)
		return false
	}
	return true
}

func oauthJSON(w http.ResponseWriter, status int, answer oauthAnswer) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(answer)
}

func (s *composerOAuth) prune() {
	for id, attempt := range s.attempts {
		if time.Now().After(attempt.expires) {
			delete(s.attempts, id)
		}
	}
}

func (s *composerOAuth) start(w http.ResponseWriter, r *http.Request) {
	if !oauthLocalRequest(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	mediaType, _, mediaErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mediaErr != nil || mediaType != "application/json" {
		http.Error(w, "expected application/json", 400)
		return
	}
	var options oauthOptions
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&options) != nil || decoder.Decode(new(any)) != io.EOF {
		http.Error(w, "invalid OAuth parameters", 400)
		return
	}
	if options.GrantType == "" {
		options.GrantType = "authorization_code"
	}
	if options.ClientID == "" || (options.GrantType != "authorization_code" && options.GrantType != "client_credentials") || (options.GrantType == "client_credentials" && options.ClientSecret == "") {
		http.Error(w, "OAuth requires clientId and a supported grantType (client_credentials also requires clientSecret)", 400)
		return
	}
	issuer, err := oauthURL(options.Issuer)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	options.Issuer = strings.TrimRight(issuer.String(), "/")
	req, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, options.Issuer+"/.well-known/openid-configuration", nil)
	response, err := s.client.Do(req)
	if err != nil {
		http.Error(w, "could not reach OAuth discovery endpoint", 502)
		return
	}
	defer response.Body.Close()
	var discovery struct {
		Issuer                string `json:"issuer"`
		AuthorizationEndpoint string `json:"authorization_endpoint"`
		TokenEndpoint         string `json:"token_endpoint"`
	}
	if response.StatusCode != 200 || json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&discovery) != nil || discovery.Issuer != options.Issuer {
		http.Error(w, "invalid OAuth discovery response or issuer mismatch", 502)
		return
	}
	if _, err := oauthURL(discovery.TokenEndpoint); err != nil {
		http.Error(w, "invalid OAuth token endpoint", 502)
		return
	}
	if options.GrantType == "client_credentials" {
		form := url.Values{"grant_type": {"client_credentials"}}
		if options.Scope != "" {
			form.Set("scope", options.Scope)
		}
		answer := s.token(r.Context(), options, discovery.TokenEndpoint, form)
		status := 200
		if answer.Error != "" {
			status = 502
		}
		oauthJSON(w, status, answer)
		return
	}
	authURL, err := oauthURL(discovery.AuthorizationEndpoint)
	if err != nil {
		http.Error(w, "invalid OAuth authorization endpoint", 502)
		return
	}
	// The result handle never travels to the identity provider; state alone cannot
	// be used to read the token from the polling endpoint.
	id, state, verifier := rand.Text(), rand.Text(), rand.Text()+rand.Text()
	redirect := oauthOrigin(r) + oauthCallbackPath
	if _, err := oauthURL(redirect); err != nil {
		http.Error(w, "invalid OAuth callback origin", 400)
		return
	}
	challenge := sha256.Sum256([]byte(verifier))
	query := authURL.Query()
	query.Set("response_type", "code")
	query.Set("client_id", options.ClientID)
	query.Set("redirect_uri", redirect)
	query.Set("state", state)
	query.Set("code_challenge", base64.RawURLEncoding.EncodeToString(challenge[:]))
	query.Set("code_challenge_method", "S256")
	if options.Scope != "" {
		query.Set("scope", options.Scope)
	}
	if options.Prompt != "" {
		query.Set("prompt", options.Prompt)
	}
	authURL.RawQuery = query.Encode()
	s.mu.Lock()
	s.prune()
	if len(s.attempts) >= 64 {
		s.mu.Unlock()
		http.Error(w, "too many pending OAuth logins", 429)
		return
	}
	s.attempts[id] = &oauthAttempt{authorizationURL: authURL.String(), options: options, tokenURL: discovery.TokenEndpoint, redirectURI: redirect, verifier: verifier, state: state, expires: time.Now().Add(oauthLifetime)}
	s.mu.Unlock()
	oauthJSON(w, 200, oauthAnswer{ID: id, AuthorizationURL: authURL.String()})
}

func (s *composerOAuth) token(ctx context.Context, options oauthOptions, endpoint string, form url.Values) oauthAnswer {
	form.Set("client_id", options.ClientID)
	// client_secret_post supports Keycloak's confidential clients; tokens and
	// secrets stay out of URLs and the traffic capture pipeline.
	if options.ClientSecret != "" {
		form.Set("client_secret", options.ClientSecret)
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := s.client.Do(req)
	if err != nil {
		return oauthAnswer{Error: "could not reach OAuth token endpoint"}
	}
	defer response.Body.Close()
	var token struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if response.StatusCode != 200 || json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&token) != nil || token.AccessToken == "" || !strings.EqualFold(token.TokenType, "Bearer") {
		return oauthAnswer{Error: "OAuth token exchange failed"}
	}
	return oauthAnswer{AccessToken: token.AccessToken, ExpiresIn: token.ExpiresIn}
}

func (s *composerOAuth) callback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
	if r.Method != http.MethodGet {
		w.WriteHeader(405)
		return
	}
	state := r.URL.Query().Get("state")
	s.mu.Lock()
	s.prune()
	var attempt *oauthAttempt
	for _, a := range s.attempts {
		if a.state == state && state != "" && !a.processing && a.answer == nil && a.redirectURI == oauthOrigin(r)+oauthCallbackPath {
			attempt = a
			break
		}
	}
	if attempt == nil {
		s.mu.Unlock()
		http.Error(w, "invalid or expired OAuth state", 400)
		return
	}
	attempt.processing = true
	s.mu.Unlock()
	answer := oauthAnswer{Error: "OAuth login was denied or returned no code"}
	if code := r.URL.Query().Get("code"); code != "" && r.URL.Query().Get("error") == "" {
		answer = s.token(r.Context(), attempt.options, attempt.tokenURL, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {attempt.redirectURI}, "code_verifier": {attempt.verifier}})
	}
	s.mu.Lock()
	attempt.answer = &answer
	attempt.verifier = ""
	attempt.options.ClientSecret = ""
	s.mu.Unlock()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, "<!doctype html><html lang=en><title>HttpStackLens OAuth</title><p>Login finished. You can close this window and return to HttpStackLens.</p></html>")
}

func (s *composerOAuth) result(w http.ResponseWriter, r *http.Request) {
	if !oauthLocalRequest(w, r) {
		return
	}
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		w.WriteHeader(405)
		return
	}
	// The unguessable handle is sent in a header to avoid URL/access logs.
	id := r.Header.Get("X-OAuth-ID")
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune()
	attempt := s.attempts[id]
	if attempt == nil {
		http.Error(w, "OAuth login expired or was cancelled", 404)
		return
	}
	if r.Method == http.MethodDelete {
		delete(s.attempts, id)
		w.WriteHeader(204)
		return
	}
	if attempt.answer == nil {
		oauthJSON(w, 202, oauthAnswer{})
		return
	}
	answer := *attempt.answer
	delete(s.attempts, id)
	oauthJSON(w, 200, answer)
}

// open only launches a URL created by discovery for a live login attempt.
// Callers cannot use this endpoint to launch arbitrary URLs or local files.
func (s *composerOAuth) open(w http.ResponseWriter, r *http.Request) {
	if !oauthLocalRequest(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	s.mu.Lock()
	s.prune()
	attempt := s.attempts[r.Header.Get("X-OAuth-ID")]
	if attempt == nil || attempt.answer != nil || attempt.processing || attempt.authorizationURL == "" {
		s.mu.Unlock()
		http.Error(w, "OAuth login is no longer available", 404)
		return
	}
	if attempt.opened {
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
		return
	}
	attempt.opened = true
	target := attempt.authorizationURL
	s.mu.Unlock()
	if s.openBrowser(target) != nil {
		s.mu.Lock()
		attempt.opened = false
		s.mu.Unlock()
		http.Error(w, "could not open the system browser for OAuth login", 500)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
