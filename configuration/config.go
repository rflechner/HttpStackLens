package configuration

import (
	"httpStackLens/webui/wasm/shared"
	"strings"
	"sync"
)

type AppConfig struct {
	Proxy        ProxyConfig        `json:"proxy"`
	WebUi        WebUiConfig        `json:"webui"`
	Logging      LoggingConfig      `yaml:"logging"`
	Storage      StorageConfig      `yaml:"storage"`
	DecryptHttps DecryptHttpsConfig `yaml:"decrypt_https"`
	Updates      UpdatesConfig      `yaml:"updates"`
}

// UpdatesConfig gates the automatic update check. It is opt-in: when CheckEnabled
// is false (the default), the app makes no outbound request to GitHub and the Web
// UI shows no update badge.
type UpdatesConfig struct {
	CheckEnabled bool `yaml:"check_enabled"` // check GitHub for a newer release on startup
}

type StorageConfig struct {
	Enable bool   `yaml:"enable"` // persist captured traffic to .capture files
	Folder string `yaml:"folder"` // destination folder (relative to cwd, or absolute)
}

// DefaultCaptureSizeBytes is the per-MIME-type body size limit used when a rule
// specifies no explicit size.
const DefaultCaptureSizeBytes int64 = 500 * 1024 // 500 KiB

type DecryptHttpsConfig struct {
	Enabled         bool              `yaml:"enabled"` // intercept & decrypt HTTPS (MITM)
	CertManager     CertManagerConfig `yaml:"cert_manager"`
	MimeTypes       []MimeTypeRule    `yaml:"mime_types"`        // per-content-type capture limits
	DefaultMaxBytes *int64            `yaml:"default_max_bytes"` // limit for content types not listed (and rules without an explicit size); defaults to DefaultCaptureSizeBytes
}

// MimeTypeRule caps how much of a response body is captured for a given content
// type. At most one of the size fields is expected; the size unit is binary
// (KiB/MiB). Pointers distinguish "unset" from an explicit zero.
type MimeTypeRule struct {
	Name         string   `yaml:"name"`           // e.g. "image/*", "text/*", "application/json"
	MaxSizeBytes *int64   `yaml:"max_size_bytes"` //
	MaxSizeKb    *float64 `yaml:"max_size_kb"`    //
	MaxSizeMb    *float64 `yaml:"max_size_mb"`    //
}

type DecryptHttpsConfigStore struct {
	mu     sync.RWMutex
	config DecryptHttpsConfig
}

func NewDecryptHttpsConfigStore(config DecryptHttpsConfig) *DecryptHttpsConfigStore {
	return &DecryptHttpsConfigStore{config: cloneDecryptHttpsConfig(config)}
}

func (s *DecryptHttpsConfigStore) Get() DecryptHttpsConfig {
	if s == nil {
		return DecryptHttpsConfig{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneDecryptHttpsConfig(s.config)
}

func (s *DecryptHttpsConfigStore) UpdateCaptureRules(defaultMaxBytes *int64, rules []MimeTypeRule) DecryptHttpsConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config.DefaultMaxBytes = cloneInt64Ptr(defaultMaxBytes)
	s.config.MimeTypes = cloneMimeTypeRules(rules)
	return cloneDecryptHttpsConfig(s.config)
}

func (s *DecryptHttpsConfigStore) UpdateEnabled(enabled bool) DecryptHttpsConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config.Enabled = enabled
	return cloneDecryptHttpsConfig(s.config)
}

func cloneDecryptHttpsConfig(config DecryptHttpsConfig) DecryptHttpsConfig {
	config.DefaultMaxBytes = cloneInt64Ptr(config.DefaultMaxBytes)
	config.MimeTypes = cloneMimeTypeRules(config.MimeTypes)
	return config
}

func cloneMimeTypeRules(rules []MimeTypeRule) []MimeTypeRule {
	if rules == nil {
		return nil
	}
	out := make([]MimeTypeRule, len(rules))
	for i, rule := range rules {
		out[i] = rule
		out[i].MaxSizeBytes = cloneInt64Ptr(rule.MaxSizeBytes)
		out[i].MaxSizeKb = cloneFloat64Ptr(rule.MaxSizeKb)
		out[i].MaxSizeMb = cloneFloat64Ptr(rule.MaxSizeMb)
	}
	return out
}

func cloneInt64Ptr(v *int64) *int64 {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

func cloneFloat64Ptr(v *float64) *float64 {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

// explicitLimit returns the size limit set by the rule's YAML, if any. The
// second result is false when the rule specifies no explicit size.
func (r MimeTypeRule) explicitLimit() (int64, bool) {
	switch {
	case r.MaxSizeBytes != nil:
		return *r.MaxSizeBytes, true
	case r.MaxSizeKb != nil:
		return int64(*r.MaxSizeKb * 1024), true
	case r.MaxSizeMb != nil:
		return int64(*r.MaxSizeMb * 1024 * 1024), true
	default:
		return 0, false
	}
}

// LimitBytes returns the body size limit in bytes, honoring whichever unit was
// set in the YAML and falling back to DefaultCaptureSizeBytes (500 KiB).
func (r MimeTypeRule) LimitBytes() int64 {
	if limit, ok := r.explicitLimit(); ok {
		return limit
	}
	return DefaultCaptureSizeBytes
}

// DefaultLimitBytes returns the limit applied to content types that match no
// rule (and to rules without an explicit size): the configured
// default_max_bytes, or DefaultCaptureSizeBytes when unset.
func (c DecryptHttpsConfig) DefaultLimitBytes() int64 {
	if c.DefaultMaxBytes != nil {
		return *c.DefaultMaxBytes
	}
	return DefaultCaptureSizeBytes
}

// LimitForContentType returns the size limit for a content type (e.g.
// "text/html; charset=utf-8"), matching rules in order with "type/*" wildcard
// support. matched reports whether any rule applied; when false the returned
// limit is DefaultLimitBytes.
func (c DecryptHttpsConfig) LimitForContentType(contentType string) (limit int64, matched bool) {
	ct := normalizeContentType(contentType)
	for _, rule := range c.MimeTypes {
		if contentTypeMatches(rule.Name, ct) {
			if explicit, ok := rule.explicitLimit(); ok {
				return explicit, true
			}
			return c.DefaultLimitBytes(), true
		}
	}
	return c.DefaultLimitBytes(), false
}

// normalizeContentType lowercases and strips any parameters ("; charset=...").
func normalizeContentType(contentType string) string {
	if i := strings.IndexByte(contentType, ';'); i >= 0 {
		contentType = contentType[:i]
	}
	return strings.ToLower(strings.TrimSpace(contentType))
}

// contentTypeMatches reports whether a normalized content type matches a rule
// name, supporting a "type/*" subtype wildcard.
func contentTypeMatches(ruleName, contentType string) bool {
	ruleName = strings.ToLower(strings.TrimSpace(ruleName))
	if ruleName == contentType || ruleName == "*/*" {
		return true
	}
	if rt, _, ok := strings.Cut(ruleName, "/"); ok {
		if st, _, ok2 := strings.Cut(contentType, "/"); ok2 {
			return strings.HasSuffix(ruleName, "/*") && rt == st
		}
	}
	return false
}

type CertManagerConfig struct {
	CaCertFile        string `yaml:"ca_cert_file"`
	CaKeyFile         string `yaml:"ca_key_file"`
	DomainCertsFolder string `yaml:"domain_certs_folder"`
}

type LoggingConfig struct {
	Level string `yaml:"level"` // debug | info | warn | error
	File  string `yaml:"file"`  // path to the log file; empty disables the file sink
}

type ProxyConfig struct {
	Port                                  int                 `yaml:"port"`
	EnableRemoteConnection                bool                `yaml:"enable_remote_connection"`
	AccessControl                         AccessControlConfig `yaml:"access_control"`
	OutputProxyUri                        string              `yaml:"output_proxy_uri"`
	AddWindowsAuthenticationToOutputProxy bool                `yaml:"add_windows_authentication_to_output_proxy"`
	Treat401AsProxyAuthentication         bool                `yaml:"treat_401_as_proxy_authentication"`
	RequireWindowsAuthentication          bool                `yaml:"require_windows_authentication"`
	NoProxy                               []string            `yaml:"no_proxy"` // hosts that bypass the upstream proxy and connect directly
}

type WebUiConfig struct {
	Port                   int                 `yaml:"port"`
	EnableRemoteConnection bool                `yaml:"enable_remote_connection"`
	AccessControl          AccessControlConfig `yaml:"access_control"`
}

func DefaultAppConfig() AppConfig {
	return AppConfig{
		Proxy:   ProxyConfig{Port: 3128, AccessControl: AccessControlConfig{Mode: AccessControlLoopback}},
		WebUi:   WebUiConfig{Port: 9000, AccessControl: AccessControlConfig{Mode: AccessControlLoopback}},
		Logging: LoggingConfig{Level: "info", File: "logs/httpStackLens.log"},
		Storage: StorageConfig{Enable: false, Folder: "captures"},
		DecryptHttps: DecryptHttpsConfig{
			Enabled:     false,
			CertManager: CertManagerConfig{CaCertFile: "debug_ca.crt", CaKeyFile: "debug_ca.key", DomainCertsFolder: "certificates/domains"},
		},
	}
}

func (c *AppConfig) ToDto() shared.AppConfigDto {
	accessControl := AccessControlSettingsFromConfig(*c)
	return shared.AppConfigDto{
		Proxy: shared.ProxyConfigDto{
			Port:                   c.Proxy.Port,
			EnableRemoteConnection: accessControl.Proxy.Mode != AccessControlLoopback,
			AccessControl:          accessControlConfigToDto(accessControl.Proxy),
		},
		WebUi: shared.WebUiConfigDto{
			Port:                   c.WebUi.Port,
			EnableRemoteConnection: accessControl.WebUi.Mode != AccessControlLoopback,
			AccessControl:          accessControlConfigToDto(accessControl.WebUi),
		},
		DecryptHttps: shared.DecryptHttpsConfigDto{
			Enabled:         c.DecryptHttps.Enabled,
			DefaultMaxBytes: c.DecryptHttps.DefaultMaxBytes,
			MimeTypes:       mimeTypeRulesToDto(c.DecryptHttps.MimeTypes),
			CertManager: shared.CertManagerConfigDto{
				CaCertFile:        c.DecryptHttps.CertManager.CaCertFile,
				CaKeyFile:         c.DecryptHttps.CertManager.CaKeyFile,
				DomainCertsFolder: c.DecryptHttps.CertManager.DomainCertsFolder,
			},
		},
	}
}

func accessControlConfigToDto(config AccessControlConfig) shared.AccessControlConfigDto {
	return shared.AccessControlConfigDto{
		Mode:     string(config.Mode),
		Networks: cloneStringSlice(config.Networks),
	}
}

func mimeTypeRulesToDto(rules []MimeTypeRule) []shared.MimeTypeRuleDto {
	if rules == nil {
		return nil
	}
	out := make([]shared.MimeTypeRuleDto, len(rules))
	for i, rule := range rules {
		out[i] = shared.MimeTypeRuleDto{
			Name:         rule.Name,
			MaxSizeBytes: rule.MaxSizeBytes,
			MaxSizeKb:    rule.MaxSizeKb,
			MaxSizeMb:    rule.MaxSizeMb,
		}
	}
	return out
}
