package middlewares

import "testing"

func TestMatchesNoProxy(t *testing.T) {
	rules := []string{"localhost", "127.0.0.1", ".local", "host.docker.internal"}

	cases := []struct {
		host string
		want bool
	}{
		{"localhost", true},               // exact
		{"127.0.0.1", true},               // exact IP
		{"host.docker.internal", true},    // exact
		{"foo.local", true},               // .suffix subdomain
		{"local", true},                   // .suffix domain itself
		{"api.github.com", false},         // no match
		{"127.0.0.2", false},              // different IP
		{"notlocalhost", false},           // not a suffix boundary
		{"my.host.docker.internal", true}, // subdomain of a plain rule
		{"LOCALHOST", true},               // case-insensitive
		{"foo.local.", true},              // trailing dot on host ignored
	}
	for _, tc := range cases {
		if got := MatchesNoProxy(tc.host, rules); got != tc.want {
			t.Errorf("MatchesNoProxy(%q) = %v, want %v", tc.host, got, tc.want)
		}
	}
}

func TestMatchesNoProxyWildcard(t *testing.T) {
	if !MatchesNoProxy("anything.example.com", []string{"*"}) {
		t.Error(`"*" should match every host`)
	}
}

func TestMatchesNoProxyCIDR(t *testing.T) {
	rules := []string{"10.0.0.0/8", "192.168.1.0/24"}
	cases := []struct {
		host string
		want bool
	}{
		{"10.1.2.3", true},
		{"192.168.1.42", true},
		{"192.168.2.42", false},
		{"172.16.0.1", false},
		{"example.com", false}, // non-IP host never matches a CIDR
	}
	for _, tc := range cases {
		if got := MatchesNoProxy(tc.host, rules); got != tc.want {
			t.Errorf("MatchesNoProxy(%q) = %v, want %v", tc.host, got, tc.want)
		}
	}
}

func TestMatchesNoProxyEmpty(t *testing.T) {
	if MatchesNoProxy("example.com", nil) {
		t.Error("no rules should never match")
	}
	if MatchesNoProxy("", []string{"localhost"}) {
		t.Error("empty host should not match")
	}
}
