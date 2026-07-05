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
	// Scheme is "http" or "https".
	Scheme string `json:"scheme"`
	// Tls is true when the request travelled over TLS (an HTTPS CONNECT tunnel or
	// a decrypted HTTPS request).
	Tls bool `json:"tls"`
	// Decrypted is true when the body/headers were MITM-decrypted and are
	// therefore inspectable (as opposed to an opaque, tunnelled CONNECT).
	Decrypted bool `json:"decrypted"`
}

// ResponseEventDto is streamed (SSE "response_occurred") when a response is
// received, carrying the same CorrelationID as its RequestEventDto so the UI can
// pair them. It is currently emitted only for decrypted HTTPS traffic; plain
// HTTP responses are piped without parsing and are not surfaced yet.
type ResponseEventDto struct {
	CorrelationID string `json:"correlation_id"`
	Status        int    `json:"status"`
	StatusText    string `json:"status_text"`
	ContentType   string `json:"content_type"`
	// Size is the number of body bytes transferred (best-effort: the actual bytes
	// streamed to the client, even when the body was not stored).
	Size int64 `json:"size"`
	// DurationMs is the elapsed time from issuing the upstream request to finishing
	// writing the response back to the client.
	DurationMs int64 `json:"duration_ms"`
	// BodyAvailable is true when the body was captured and can be fetched for the
	// detail view; BodySkipped is true when it was dropped for exceeding its
	// per-content-type size limit.
	BodyAvailable bool `json:"body_available"`
	BodySkipped   bool `json:"body_skipped"`
	// Stream is true for streaming responses (SSE, WebSocket upgrade) whose body is
	// never captured — only frame metadata.
	Stream bool `json:"stream"`
}

// RequestDetailDto is returned by GET /api/requests/{id}. It contains metadata
// and headers only; bodies are fetched separately by /api/requests/{id}/body.
type RequestDetailDto struct {
	CorrelationID string                    `json:"correlation_id"`
	CreatedAt     string                    `json:"created_at"`
	Request       *RequestDetailRequestDto  `json:"request,omitempty"`
	Response      *RequestDetailResponseDto `json:"response,omitempty"`
}

type RequestDetailRequestDto struct {
	Method        string      `json:"method"`
	URL           string      `json:"url"`
	HttpVersion   string      `json:"http_version"`
	Headers       []HeaderDto `json:"headers"`
	BodyAvailable bool        `json:"body_available"`
	BodySkipped   bool        `json:"body_skipped"`
	BodySize      int         `json:"body_size"`
}

type RequestDetailResponseDto struct {
	Status        int         `json:"status"`
	StatusText    string      `json:"status_text"`
	HttpVersion   string      `json:"http_version"`
	Headers       []HeaderDto `json:"headers"`
	BodyAvailable bool        `json:"body_available"`
	BodySkipped   bool        `json:"body_skipped"`
	BodySize      int         `json:"body_size"`
}

type HeaderDto struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type CertificatesInfosDto struct {
	CaCertSubject string `json:"ca_cert_subject"`
}

// duplicate of AppConfig to avoid security issues

type AppConfigDto struct {
	Proxy        ProxyConfigDto        `json:"proxy"`
	WebUi        WebUiConfigDto        `json:"web_ui"`
	DecryptHttps DecryptHttpsConfigDto `json:"decrypt_https"`
}

type ProxyConfigDto struct {
	Port                   int  `json:"port"`
	EnableRemoteConnection bool `json:"enable_remote_connection"`
}

type WebUiConfigDto struct {
	Port                   int  `json:"port"`
	EnableRemoteConnection bool `json:"enable_remote_connection"`
}

type DecryptHttpsConfigDto struct {
	Enabled     bool                 `json:"enabled"`
	CertManager CertManagerConfigDto `json:"cert_manager"`
}

type CertManagerConfigDto struct {
	CaCertFile        string `json:"ca_cert_file"`
	CaKeyFile         string `json:"ca_key_file"`
	DomainCertsFolder string `json:"domain_certs_folder"`
}
