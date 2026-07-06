package configuration

import (
	"strings"
	"sync"
)

// UpstreamSettings holds the outbound (corporate) proxy configuration that the
// Web UI can read and edit at runtime (B5.2). It is a focused subset of
// ProxyConfig: the upstream URL, the direct-connection bypass list, and the
// Windows authentication toggle.
type UpstreamSettings struct {
	OutputProxyUri           string
	NoProxy                  []string
	AddWindowsAuthentication bool
}

// UpstreamSettingsFromProxyConfig extracts the upstream-related fields from the
// full proxy configuration.
func UpstreamSettingsFromProxyConfig(config ProxyConfig) UpstreamSettings {
	return UpstreamSettings{
		OutputProxyUri:           config.OutputProxyUri,
		NoProxy:                  cloneStringSlice(config.NoProxy),
		AddWindowsAuthentication: config.AddWindowsAuthenticationToOutputProxy,
	}
}

// UpstreamSettingsStore keeps the current upstream proxy settings behind a mutex
// so the Web UI reads the live value (including edits made this session) rather
// than the startup snapshot. Hot re-injection into the running pipeline is out
// of scope; edits take effect on the next application start.
type UpstreamSettingsStore struct {
	mu       sync.RWMutex
	settings UpstreamSettings
}

func NewUpstreamSettingsStore(settings UpstreamSettings) *UpstreamSettingsStore {
	return &UpstreamSettingsStore{settings: cloneUpstreamSettings(settings)}
}

func (s *UpstreamSettingsStore) Get() UpstreamSettings {
	if s == nil {
		return UpstreamSettings{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneUpstreamSettings(s.settings)
}

func (s *UpstreamSettingsStore) Update(settings UpstreamSettings) UpstreamSettings {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings = cloneUpstreamSettings(settings)
	return cloneUpstreamSettings(s.settings)
}

func cloneUpstreamSettings(settings UpstreamSettings) UpstreamSettings {
	settings.OutputProxyUri = strings.TrimSpace(settings.OutputProxyUri)
	settings.NoProxy = cloneStringSlice(settings.NoProxy)
	return settings
}

func cloneStringSlice(values []string) []string {
	if values == nil {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}
