package configuration

import (
	"net"
	"net/netip"
	"testing"
)

func TestNormalizeAccessControlFallsBackToLegacyRemoteFlag(t *testing.T) {
	if got := NormalizeAccessControl(AccessControlConfig{}, false).Mode; got != AccessControlLoopback {
		t.Fatalf("legacy false mode = %q, want loopback", got)
	}
	if got := NormalizeAccessControl(AccessControlConfig{}, true).Mode; got != AccessControlOpen {
		t.Fatalf("legacy true mode = %q, want open", got)
	}
}

func TestAccessPolicyAllowsExpectedIPs(t *testing.T) {
	cases := []struct {
		name   string
		config AccessControlConfig
		ip     string
		want   bool
	}{
		{"loopback allows 127", AccessControlConfig{Mode: AccessControlLoopback}, "127.0.0.1", true},
		{"loopback rejects lan", AccessControlConfig{Mode: AccessControlLoopback}, "192.168.1.10", false},
		{"lan allows private", AccessControlConfig{Mode: AccessControlLan}, "10.1.2.3", true},
		{"lan allows loopback", AccessControlConfig{Mode: AccessControlLan}, "::1", true},
		{"lan rejects public", AccessControlConfig{Mode: AccessControlLan}, "8.8.8.8", false},
		{"allowlist allows network", AccessControlConfig{Mode: AccessControlAllowlist, Networks: []string{"203.0.113.0/24"}}, "203.0.113.5", true},
		{"allowlist rejects other", AccessControlConfig{Mode: AccessControlAllowlist, Networks: []string{"203.0.113.0/24"}}, "203.0.114.5", false},
		{"open allows public", AccessControlConfig{Mode: AccessControlOpen}, "8.8.8.8", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ip := netip.MustParseAddr(tc.ip)
			if got := NewAccessPolicy(tc.config).AllowsIP(ip); got != tc.want {
				t.Fatalf("AllowsIP(%s) = %v, want %v", tc.ip, got, tc.want)
			}
		})
	}
}

func TestAccessPolicyAllowsTCPAddr(t *testing.T) {
	addr := &net.TCPAddr{IP: net.ParseIP("192.168.1.42"), Port: 50000}
	policy := NewAccessPolicy(AccessControlConfig{Mode: AccessControlLan})
	if !policy.AllowsAddr(addr) {
		t.Fatal("LAN policy should allow private TCPAddr")
	}
}

func TestValidateAccessControlRejectsInvalidNetwork(t *testing.T) {
	_, err := ValidateAccessControl(AccessControlConfig{
		Mode:     AccessControlAllowlist,
		Networks: []string{"not-network"},
	})
	if err == nil {
		t.Fatal("expected invalid network error")
	}
}

func TestValidateAccessControlRequiresAllowlistNetwork(t *testing.T) {
	_, err := ValidateAccessControl(AccessControlConfig{Mode: AccessControlAllowlist})
	if err == nil {
		t.Fatal("expected missing allowlist network error")
	}
}

func TestAccessControlListenHostDoesNotOpenForInvalidMode(t *testing.T) {
	got := (AccessControlConfig{Mode: AccessControlMode("typo")}).ListenHost()
	if got != "127.0.0.1" {
		t.Fatalf("ListenHost invalid mode = %q, want 127.0.0.1", got)
	}
}
