package configuration

import (
	"httpStackLens/webui/wasm/shared"
	"log"
	"os"
	"strings"

	"github.com/goccy/go-yaml"
)

type AppConfig struct {
	Proxy       ProxyConfig       `json:"proxy"`
	WebUi       WebUiConfig       `json:"webui"`
	CertManager CertManagerConfig `json:"cert_manager"`
	Logging     LoggingConfig     `yaml:"logging"`
	Storage     StorageConfig     `yaml:"storage"`
	Capture     CaptureConfig     `yaml:"capture"`
}

type StorageConfig struct {
	Enable bool   `yaml:"enable"` // persist captured traffic to .capture files
	Folder string `yaml:"folder"` // destination folder (relative to cwd, or absolute)
}

// DefaultCaptureSizeBytes is the per-MIME-type body size limit used when a rule
// specifies no explicit size.
const DefaultCaptureSizeBytes int64 = 500 * 1024 // 500 KiB

type CaptureConfig struct {
	DecryptHttps    bool           `yaml:"decrypt_https"`     // intercept & decrypt HTTPS (MITM)
	MimeTypes       []MimeTypeRule `yaml:"mime_types"`        // per-content-type capture limits
	DefaultMaxBytes *int64         `yaml:"default_max_bytes"` // limit for content types not listed (and rules without an explicit size); defaults to DefaultCaptureSizeBytes
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
func (c CaptureConfig) DefaultLimitBytes() int64 {
	if c.DefaultMaxBytes != nil {
		return *c.DefaultMaxBytes
	}
	return DefaultCaptureSizeBytes
}

// LimitForContentType returns the size limit for a content type (e.g.
// "text/html; charset=utf-8"), matching rules in order with "type/*" wildcard
// support. matched reports whether any rule applied; when false the returned
// limit is DefaultLimitBytes.
func (c CaptureConfig) LimitForContentType(contentType string) (limit int64, matched bool) {
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
	Port                                  int      `yaml:"port"`
	EnableRemoteConnection                bool     `yaml:"enable_remote_connection"`
	OutputProxyUri                        string   `yaml:"output_proxy_uri"`
	AddWindowsAuthenticationToOutputProxy bool     `yaml:"add_windows_authentication_to_output_proxy"`
	Treat401AsProxyAuthentication         bool     `yaml:"treat_401_as_proxy_authentication"`
	RequireWindowsAuthentication          bool     `yaml:"require_windows_authentication"`
	NoProxy                               []string `yaml:"no_proxy"` // hosts that bypass the upstream proxy and connect directly
}

type WebUiConfig struct {
	Port                   int  `yaml:"port"`
	EnableRemoteConnection bool `yaml:"enable_remote_connection"`
}

func DefaultAppConfig() AppConfig {
	return AppConfig{
		Proxy:       ProxyConfig{Port: 3128, EnableRemoteConnection: false},
		WebUi:       WebUiConfig{Port: 9000, EnableRemoteConnection: false},
		CertManager: CertManagerConfig{CaCertFile: "debug_ca.crt", CaKeyFile: "debug_ca.key", DomainCertsFolder: "certificates/domains"},
		Logging:     LoggingConfig{Level: "info", File: "logs/httpStackLens.log"},
		Storage:     StorageConfig{Enable: false, Folder: "captures"},
		Capture:     CaptureConfig{DecryptHttps: false},
	}
}

func ReadConfiguration() AppConfig {
	configData, err := os.ReadFile("config.yaml")
	if err != nil {
		log.Printf("Failed to parse configuration file: %v\n", err)
		return DefaultAppConfig()
	}
	var conf AppConfig
	err = yaml.Unmarshal(configData, &conf)
	if err != nil {
		log.Printf("Failed to parse configuration file: %v\n", err)
		return DefaultAppConfig()
	}

	return conf
}

func (c *AppConfig) ToDto() shared.AppConfigDto {
	return shared.AppConfigDto{
		Proxy: shared.ProxyConfigDto{
			Port:                   c.Proxy.Port,
			EnableRemoteConnection: c.Proxy.EnableRemoteConnection,
		},
		WebUi: shared.WebUiConfigDto{
			Port:                   c.WebUi.Port,
			EnableRemoteConnection: c.WebUi.EnableRemoteConnection,
		},
		CertManager: shared.CertManagerConfigDto{
			CaCertFile:        c.CertManager.CaCertFile,
			CaKeyFile:         c.CertManager.CaKeyFile,
			DomainCertsFolder: c.CertManager.DomainCertsFolder,
		},
	}
}
