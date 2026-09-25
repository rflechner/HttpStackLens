package configuration

import (
	"errors"
	"net"
	"net/netip"
	"testing"
)

func TestInterfaceAccessPolicyAllowsOnlyAttachedSubnets(t *testing.T) {
	policy := NewAccessPolicy(AccessControlConfig{
		Mode: AccessControlInterfaces,
		// Manually configured networks apply only to allowlist mode.
		Networks: []string{"8.8.8.0/24"},
	})
	policy.interfaceAddrs = func() ([]net.Addr, error) {
		var addrs []net.Addr
		for _, cidr := range []string{
			"192.168.1.50/24", "172.17.0.1/16", "172.28.112.1/20",
			"100.64.12.1/24", "203.0.113.10/27", "2001:db8:1::1/64",
			"fe80::1234/64", "10.42.0.1/32", "fd00::1/128",
		} {
			ip, network, err := net.ParseCIDR(cidr)
			if err != nil {
				t.Fatal(err)
			}
			network.IP = ip // OS APIs return the adapter address, not the subnet base.
			addrs = append(addrs, network)
		}
		return addrs, nil
	}
	cases := []struct {
		name string
		ip   string
		want bool
	}{
		{"IPv4 localhost", "127.0.0.1", true},
		{"IPv6 localhost", "::1", true},
		{"physical LAN", "192.168.1.42", true},
		{"Docker bridge", "172.17.2.3", true},
		{"Hyper-V or WSL subnet", "172.28.127.254", true},
		{"outside WSL mask", "172.28.128.1", false},
		{"VPN outside RFC1918", "100.64.12.42", true},
		{"public adapter subnet", "203.0.113.30", true},
		{"outside public adapter mask", "203.0.113.32", false},
		{"IPv6 subnet", "2001:db8:1::42", true},
		{"other IPv6 subnet", "2001:db8:2::42", false},
		{"scoped IPv6", "fe80::42%eth0", true},
		{"mapped IPv4", "::ffff:172.17.2.3", true},
		{"IPv4 host route", "10.42.0.1", true},
		{"outside IPv4 host route", "10.42.0.2", false},
		{"IPv6 host route", "fd00::1", true},
		{"outside IPv6 host route", "fd00::2", false},
		{"unattached private subnet", "10.99.0.1", false},
		{"unattached public subnet", "8.8.8.8", false},
		{"IPv4 unspecified", "0.0.0.0", false},
		{"IPv6 unspecified", "::", false},
		{"multicast", "ff02::1", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := policy.AllowsIP(netip.MustParseAddr(tc.ip)); got != tc.want {
				t.Fatalf("AllowsIP(%s) = %v, want %v", tc.ip, got, tc.want)
			}
		})
	}
	if policy.AllowsIP(netip.Addr{}) || policy.AllowsAddr(nil) {
		t.Fatal("invalid or missing client addresses must be denied")
	}
	if !policy.AllowsAddr(&net.TCPAddr{IP: net.ParseIP("fe80::42"), Zone: "eth0", Port: 50000}) {
		t.Fatal("scoped IPv6 TCP clients should match the interface subnet")
	}
}

func TestInterfaceAccessPolicyRefreshesAndFailsClosed(t *testing.T) {
	policy := NewAccessPolicy(AccessControlConfig{Mode: AccessControlInterfaces})
	_, network, _ := net.ParseCIDR("172.17.0.1/16")
	var addrs []net.Addr
	var lookupErr error
	policy.interfaceAddrs = func() ([]net.Addr, error) { return addrs, lookupErr }
	client := netip.MustParseAddr("172.17.0.2")
	if policy.AllowsIP(client) {
		t.Fatal("empty interface list must deny remote clients")
	}
	addrs = []net.Addr{network}
	if !policy.AllowsIP(client) {
		t.Fatal("newly attached container subnet should be allowed")
	}
	lookupErr = errors.New("interface discovery failed")
	if policy.AllowsIP(client) {
		t.Fatal("discovery error must deny remote clients, even with partial results")
	}
	if !policy.AllowsIP(netip.MustParseAddr("127.0.0.1")) || !policy.AllowsIP(netip.IPv6Loopback()) {
		t.Fatal("discovery error must keep localhost reachable")
	}
	lookupErr = nil
	addrs = nil
	if policy.AllowsIP(client) {
		t.Fatal("removed container subnet must no longer be allowed")
	}
}

func TestInterfaceAccessPolicyIgnoresUnsafeInterfaceAddresses(t *testing.T) {
	policy := NewAccessPolicy(AccessControlConfig{Mode: AccessControlInterfaces})
	policy.interfaceAddrs = func() ([]net.Addr, error) {
		return []net.Addr{
			nil, (*net.IPNet)(nil), &net.IPAddr{IP: net.ParseIP("192.168.1.1")},
			&net.IPNet{IP: net.ParseIP("192.168.1.1"), Mask: net.CIDRMask(0, 32)},
			&net.IPNet{IP: net.ParseIP("2001:db8::1"), Mask: net.CIDRMask(0, 128)},
			&net.IPNet{IP: net.ParseIP("10.0.0.1"), Mask: net.IPMask{255, 0, 255, 0}},
			&net.IPNet{IP: net.IPv4zero, Mask: net.CIDRMask(8, 32)},
			&net.IPNet{IP: net.ParseIP("224.0.0.1"), Mask: net.CIDRMask(3, 32)},
		}, nil
	}
	for _, ip := range []string{"192.168.1.2", "2001:db8::42", "10.1.0.2", "0.1.2.3", "240.0.0.1"} {
		if policy.AllowsIP(netip.MustParseAddr(ip)) {
			t.Fatalf("unsafe interface address should not allow %s", ip)
		}
	}
}

func TestInterfaceAccessPolicyUsesSystemInterfaces(t *testing.T) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		t.Skipf("system interface discovery unavailable in this environment: %v", err)
	}
	policy := NewAccessPolicy(AccessControlConfig{Mode: AccessControlInterfaces})
	for _, addr := range addrs {
		network, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		ip, ok := netip.AddrFromSlice(network.IP)
		if !ok || ip.IsUnspecified() || ip.IsMulticast() {
			continue
		}
		ones, _ := network.Mask.Size()
		if ones == 0 {
			continue
		}
		if !policy.AllowsIP(ip) {
			t.Fatalf("system adapter address %s should be allowed", ip)
		}
	}
}

func TestValidateInterfaceAccessControlAndListenHost(t *testing.T) {
	config, err := ValidateAccessControl(AccessControlConfig{Mode: AccessControlInterfaces})
	if err != nil {
		t.Fatalf("interfaces mode should not require an explicit network: %v", err)
	}
	if config.Mode != AccessControlInterfaces || config.ListenHost() != "0.0.0.0" {
		t.Fatalf("interfaces mode must listen for remote clients: %+v", config)
	}
}

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
		{"allowlist always allows IPv4 loopback", AccessControlConfig{Mode: AccessControlAllowlist, Networks: []string{"203.0.113.0/24"}}, "127.0.0.1", true},
		{"allowlist always allows IPv6 loopback", AccessControlConfig{Mode: AccessControlAllowlist, Networks: []string{"203.0.113.0/24"}}, "::1", true},
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

func TestValidateAccessControlNormalizesIndividualIPs(t *testing.T) {
	got, err := ValidateAccessControl(AccessControlConfig{
		Mode:     AccessControlAllowlist,
		Networks: []string{"127.0.0.1", "::1"},
	})
	if err != nil {
		t.Fatalf("ValidateAccessControl: %v", err)
	}
	want := []string{"127.0.0.1/32", "::1/128"}
	if len(got.Networks) != len(want) {
		t.Fatalf("networks = %v, want %v", got.Networks, want)
	}
	for i := range want {
		if got.Networks[i] != want[i] {
			t.Fatalf("networks[%d] = %q, want %q", i, got.Networks[i], want[i])
		}
	}
}

func TestAccessPolicyAcceptsIndividualIPInConfig(t *testing.T) {
	policy := NewAccessPolicy(AccessControlConfig{
		Mode:     AccessControlAllowlist,
		Networks: []string{"192.168.1.42"},
	})
	if !policy.AllowsIP(netip.MustParseAddr("192.168.1.42")) {
		t.Fatal("allowlist should accept an individual IP from config")
	}
	if policy.AllowsIP(netip.MustParseAddr("192.168.1.43")) {
		t.Fatal("individual IP should not allow adjacent addresses")
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

func TestAccessInterfacePrefixPreservesAddressAndSubnet(t *testing.T) {
	for _, value := range []string{"192.168.42.17/24", "2001:db8::42/64"} {
		ip, network, err := net.ParseCIDR(value)
		if err != nil {
			t.Fatal(err)
		}
		network.IP = ip
		prefix, ok := AccessInterfacePrefix(network)
		if !ok || prefix.String() != value {
			t.Fatalf("interface address = %v, %v; want %s", prefix, ok, value)
		}
		if prefix.Masked() != netip.MustParsePrefix(value).Masked() {
			t.Fatalf("unexpected allowed subnet: %s", prefix.Masked())
		}
	}
}
