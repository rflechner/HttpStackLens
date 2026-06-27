package configuration

import (
	"httpStackLens/webui/wasm/shared"
	"log"
	"os"

	"github.com/goccy/go-yaml"
)

type AppConfig struct {
	Proxy       ProxyConfig       `json:"proxy"`
	WebUi       WebUiConfig       `json:"webui"`
	CertManager CertManagerConfig `json:"cert_manager"`
    Logging LoggingConfig `yaml:"logging"`
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
	Port                                  int    `yaml:"port"`
	EnableRemoteConnection                bool   `yaml:"enable_remote_connection"`
	OutputProxyUri                        string `yaml:"output_proxy_uri"`
	AddWindowsAuthenticationToOutputProxy bool   `yaml:"add_windows_authentication_to_output_proxy"`
	Treat401AsProxyAuthentication         bool   `yaml:"treat_401_as_proxy_authentication"`
	RequireWindowsAuthentication          bool   `yaml:"require_windows_authentication"`
	DecryptHttps                          bool   `yaml:"decrypt_https"`
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
		Logging: LoggingConfig{Level: "info", File: "logs/httpStackLens.log"},
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
