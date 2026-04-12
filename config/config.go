package configuration

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
