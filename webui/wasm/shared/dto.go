package shared

type RequestEventDto struct {
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
	Path    string `json:"path"`
	Version string `json:"version"`
}

// duplicate of AppConfig to avoid security issues

type AppConfigDto struct {
	Proxy ProxyConfigDto `json:"proxy"`
	WebUi WebUiConfigDto `json:"web_ui"`
}

type ProxyConfigDto struct {
	Port                   int  `json:"port"`
	EnableRemoteConnection bool `json:"enable_remote_connection"`
}

type WebUiConfigDto struct {
	Port                   int  `json:"port"`
	EnableRemoteConnection bool `json:"enable_remote_connection"`
}
