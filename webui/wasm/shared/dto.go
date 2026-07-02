package shared

type RequestEventDto struct {
	// ID is the incrementing per-connection sequence number, kept for display
	// (the "#001" column) and ordering in the UI.
	ID int `json:"id"`
	// CorrelationID is the stable UUID shared with the matching response event,
	// so request↔response can be linked in the UI. It is also the RequestID of
	// the capture record persisted for this request.
	CorrelationID string `json:"correlation_id"`
	Method        string `json:"method"`
	Host          string `json:"host"`
	Port          int    `json:"port"`
	Path          string `json:"path"`
	Version       string `json:"version"`
}

type CertificatesInfosDto struct {
	CaCertSubject string `json:"ca_cert_subject"`
}

// duplicate of AppConfig to avoid security issues

type AppConfigDto struct {
	Proxy       ProxyConfigDto       `json:"proxy"`
	WebUi       WebUiConfigDto       `json:"web_ui"`
	CertManager CertManagerConfigDto `json:"cert_manager"`
}

type ProxyConfigDto struct {
	Port                   int  `json:"port"`
	EnableRemoteConnection bool `json:"enable_remote_connection"`
}

type WebUiConfigDto struct {
	Port                   int  `json:"port"`
	EnableRemoteConnection bool `json:"enable_remote_connection"`
}

type CertManagerConfigDto struct {
	CaCertFile        string `yaml:"ca_cert_file"`
	CaKeyFile         string `yaml:"ca_key_file"`
	DomainCertsFolder string `yaml:"domain_certs_folder"`
}
