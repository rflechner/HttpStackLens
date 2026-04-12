package configuration

import (
	"log"
	"os"

	"github.com/goccy/go-yaml"
)

type AppConfig struct {
	Proxy ProxyConfig
	WebUi WebUiConfig
}

type ProxyConfig struct {
	Port                   int  `yaml:"port"`
	EnableRemoteConnection bool `yaml:"enable_remote_connection"`
}

type WebUiConfig struct {
	Port                   int  `yaml:"port"`
	EnableRemoteConnection bool `yaml:"enable_remote_connection"`
}

func DefaultAppConfig() AppConfig {
	return AppConfig{
		Proxy: ProxyConfig{Port: 3128, EnableRemoteConnection: false},
		WebUi: WebUiConfig{Port: 9000, EnableRemoteConnection: false},
	}
}

func ReadConfiguration() AppConfig {
	configData, err := os.ReadFile("configuration.yaml")
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
