package middlewares

import (
	"httpStackLens/http/models"
	"testing"
)

func TestDetectUpstreamAuthChallengeUsesProxyAuthenticationFor407(t *testing.T) {
	middleware := ForwardProxyServerWithWindowsAuthentication{}

	challenge, ok := middleware.detectUpstreamAuthChallenge(models.HttpResponseHead{StatusCode: 407})

	if !ok {
		t.Fatal("expected 407 to be detected as an upstream auth challenge")
	}
	if challenge.authenticateHeader != "Proxy-Authenticate" {
		t.Fatalf("expected Proxy-Authenticate header, got %s", challenge.authenticateHeader)
	}
	if challenge.authorizationHeader != "Proxy-Authorization" {
		t.Fatalf("expected Proxy-Authorization header, got %s", challenge.authorizationHeader)
	}
}

func TestDetectUpstreamAuthChallengeIgnores401ByDefault(t *testing.T) {
	middleware := ForwardProxyServerWithWindowsAuthentication{}

	_, ok := middleware.detectUpstreamAuthChallenge(models.HttpResponseHead{StatusCode: 401})

	if ok {
		t.Fatal("expected 401 to be ignored by default")
	}
}

func TestDetectUpstreamAuthChallengeUsesWwwAuthenticationFor401CompatibilityMode(t *testing.T) {
	middleware := ForwardProxyServerWithWindowsAuthentication{
		Treat401AsProxyAuthentication: true,
	}

	challenge, ok := middleware.detectUpstreamAuthChallenge(models.HttpResponseHead{StatusCode: 401})

	if !ok {
		t.Fatal("expected 401 to be detected in compatibility mode")
	}
	if challenge.authenticateHeader != "WWW-Authenticate" {
		t.Fatalf("expected WWW-Authenticate header, got %s", challenge.authenticateHeader)
	}
	if challenge.authorizationHeader != "Authorization" {
		t.Fatalf("expected Authorization header, got %s", challenge.authorizationHeader)
	}
}
