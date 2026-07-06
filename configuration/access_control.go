package configuration

import (
	"fmt"
	"net"
	"net/netip"
	"strings"
	"sync"
)

type AccessControlMode string

const (
	AccessControlLoopback  AccessControlMode = "loopback"
	AccessControlLan       AccessControlMode = "lan"
	AccessControlAllowlist AccessControlMode = "allowlist"
	AccessControlOpen      AccessControlMode = "open"
)

type AccessControlConfig struct {
	Mode     AccessControlMode `yaml:"mode"`
	Networks []string          `yaml:"networks"`
}

type AccessControlSettings struct {
	Proxy AccessControlConfig
	WebUi AccessControlConfig
}

type AccessControlSettingsStore struct {
	mu       sync.RWMutex
	settings AccessControlSettings
}

func NewAccessControlSettingsStore(settings AccessControlSettings) *AccessControlSettingsStore {
	return &AccessControlSettingsStore{settings: cloneAccessControlSettings(settings)}
}

func (s *AccessControlSettingsStore) Get() AccessControlSettings {
	if s == nil {
		return AccessControlSettings{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneAccessControlSettings(s.settings)
}

func (s *AccessControlSettingsStore) Update(settings AccessControlSettings) AccessControlSettings {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings = cloneAccessControlSettings(settings)
	return cloneAccessControlSettings(s.settings)
}

func (s *AccessControlSettingsStore) AllowsProxy(addr net.Addr) bool {
	return NewAccessPolicy(s.Get().Proxy).AllowsAddr(addr)
}

func (s *AccessControlSettingsStore) AllowsWebUi(addr net.Addr) bool {
	return NewAccessPolicy(s.Get().WebUi).AllowsAddr(addr)
}

func AccessControlSettingsFromConfig(config AppConfig) AccessControlSettings {
	return AccessControlSettings{
		Proxy: NormalizeAccessControl(config.Proxy.AccessControl, config.Proxy.EnableRemoteConnection),
		WebUi: NormalizeAccessControl(config.WebUi.AccessControl, config.WebUi.EnableRemoteConnection),
	}
}

func NormalizeAccessControl(config AccessControlConfig, enableRemoteConnection bool) AccessControlConfig {
	config.Mode = AccessControlMode(strings.TrimSpace(string(config.Mode)))
	config.Networks = cleanNetworks(config.Networks)
	if config.Mode == "" {
		if enableRemoteConnection {
			config.Mode = AccessControlOpen
		} else {
			config.Mode = AccessControlLoopback
		}
	}
	return config
}

func ValidateAccessControl(config AccessControlConfig) (AccessControlConfig, error) {
	config.Mode = AccessControlMode(strings.TrimSpace(string(config.Mode)))
	config.Networks = cleanNetworks(config.Networks)
	switch config.Mode {
	case AccessControlLoopback, AccessControlLan, AccessControlOpen:
	case AccessControlAllowlist:
		if len(config.Networks) == 0 {
			return AccessControlConfig{}, fmt.Errorf("allowlist mode requires at least one network")
		}
	default:
		return AccessControlConfig{}, fmt.Errorf("mode must be one of loopback, lan, allowlist, or open")
	}
	for _, network := range config.Networks {
		if _, err := netip.ParsePrefix(network); err != nil {
			return AccessControlConfig{}, fmt.Errorf("invalid network %q: must be CIDR notation: %v", network, err)
		}
	}
	return config, nil
}

type AccessPolicy struct {
	mode     AccessControlMode
	prefixes []netip.Prefix
}

func NewAccessPolicy(config AccessControlConfig) AccessPolicy {
	config = NormalizeAccessControl(config, false)
	policy := AccessPolicy{mode: config.Mode}
	for _, network := range config.Networks {
		prefix, err := netip.ParsePrefix(network)
		if err == nil {
			policy.prefixes = append(policy.prefixes, prefix)
		}
	}
	return policy
}

func (p AccessPolicy) AllowsAddr(addr net.Addr) bool {
	if addr == nil {
		return false
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		host = addr.String()
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	return p.AllowsIP(ip)
}

func (p AccessPolicy) AllowsIP(ip netip.Addr) bool {
	if ip.Is4In6() {
		ip = ip.Unmap()
	}
	switch p.mode {
	case AccessControlOpen:
		return true
	case AccessControlLan:
		return ip.IsLoopback() || ip.IsPrivate()
	case AccessControlAllowlist:
		for _, prefix := range p.prefixes {
			if prefix.Contains(ip) {
				return true
			}
		}
		return false
	case AccessControlLoopback, "":
		return ip.IsLoopback()
	default:
		return false
	}
}

func (c AccessControlConfig) ListenHost() string {
	switch NormalizeAccessControl(c, false).Mode {
	case AccessControlLan, AccessControlAllowlist, AccessControlOpen:
		return "0.0.0.0"
	default:
		return "127.0.0.1"
	}
}

func cloneAccessControlSettings(settings AccessControlSettings) AccessControlSettings {
	settings.Proxy = cloneAccessControlConfig(settings.Proxy)
	settings.WebUi = cloneAccessControlConfig(settings.WebUi)
	return settings
}

func cloneAccessControlConfig(config AccessControlConfig) AccessControlConfig {
	config.Networks = cloneStringSlice(config.Networks)
	return config
}

func cleanNetworks(values []string) []string {
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}
