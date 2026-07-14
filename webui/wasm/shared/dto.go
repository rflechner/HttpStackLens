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
	// Timing is the per-phase breakdown of the exchange (B4). It is present only
	// for decrypted HTTPS traffic, where the transport is instrumented.
	Timing *TimingDto `json:"timing,omitempty"`
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

// TimingDto carries the per-phase breakdown of an exchange, in milliseconds, so
// the UI can draw a real waterfall / Timing tab instead of fake ratios. Phases
// that did not occur — DNS/connect/TLS on a reused keep-alive connection — are
// zero.
type TimingDto struct {
	DnsMs      int64 `json:"dns_ms"`
	ConnectMs  int64 `json:"connect_ms"`
	TlsMs      int64 `json:"tls_ms"`
	TtfbMs     int64 `json:"ttfb_ms"`
	DownloadMs int64 `json:"download_ms"`
	TotalMs    int64 `json:"total_ms"`
}

type HeaderDto struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type CaptureFileDto struct {
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	ModifiedAt string `json:"modified_at"`
}

type CaptureMetadataDto struct {
	Name           string `json:"name"`
	Size           int64  `json:"size"`
	ModifiedAt     string `json:"modified_at"`
	Version        int16  `json:"version"`
	HttpsDecrypted bool   `json:"https_decrypted"`
	RecordsCount   int32  `json:"records_count"`
}

type CaptureRecordsDto struct {
	Name       string             `json:"name"`
	Offset     int                `json:"offset"`
	Limit      int                `json:"limit"`
	Records    []CaptureRecordDto `json:"records"`
	NextOffset int                `json:"next_offset"`
	HasMore    bool               `json:"has_more"`
}

type CaptureRecordDto struct {
	Index    int                       `json:"index"`
	Type     string                    `json:"type"`
	Request  *CaptureRequestRecordDto  `json:"request,omitempty"`
	Response *CaptureResponseRecordDto `json:"response,omitempty"`
}

type CaptureRequestRecordDto struct {
	RequestID     string      `json:"request_id"`
	Method        string      `json:"method"`
	URL           string      `json:"url"`
	HttpVersion   string      `json:"http_version"`
	Headers       []HeaderDto `json:"headers"`
	BodySkipped   bool        `json:"body_skipped"`
	BodyAvailable bool        `json:"body_available"`
	BodySize      int         `json:"body_size"`
	BodyBase64    string      `json:"body_base64,omitempty"`
}

type CaptureResponseRecordDto struct {
	RequestID     string      `json:"request_id"`
	Status        int         `json:"status"`
	StatusText    string      `json:"status_text"`
	HttpVersion   string      `json:"http_version"`
	Headers       []HeaderDto `json:"headers"`
	BodySkipped   bool        `json:"body_skipped"`
	BodyAvailable bool        `json:"body_available"`
	BodySize      int         `json:"body_size"`
	BodyBase64    string      `json:"body_base64,omitempty"`
}

type CaptureStateDto struct {
	// Capturing is kept for older UI clients. Recording is the preferred name:
	// it gates inspection/storage but never controls proxy forwarding.
	Capturing  bool                    `json:"capturing"`
	Recording  bool                    `json:"recording"`
	BufferSize int                     `json:"buffer_size"`
	Proxy      ProxyRuntimeStateDto    `json:"proxy"`
	Decrypt    CaptureDecryptStateDto  `json:"decrypt"`
	Upstream   CaptureUpstreamStateDto `json:"upstream"`
	Access     CaptureAccessStateDto   `json:"access"`
}

type ProxyRuntimeStateDto struct {
	Running bool   `json:"running"`
	Address string `json:"address"`
}

// RuntimeStatsDto contains lightweight process metrics displayed by the Web UI.
// MemoryBytes is the total memory currently obtained from the operating system
// by the Go runtime (runtime.MemStats.Sys).
type RuntimeStatsDto struct {
	MemoryBytes uint64 `json:"memory_bytes"`
}

// BuildInfoDto reports the build metadata shown in the Web UI status bar.
// CommitURL links to the exact commit on GitHub; it is empty for local/dev
// builds where the commit hash is unknown.
type BuildInfoDto struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Date      string `json:"date"`
	CommitURL string `json:"commit_url"`
}

// CaptureDecryptStateDto reports whether HTTPS decryption (MITM) is currently on,
// so the status bar can show "decrypted" vs "passthrough".
type CaptureDecryptStateDto struct {
	Enabled bool `json:"enabled"`
}

// CaptureUpstreamStateDto summarizes the outbound proxy state for the status bar:
// Enabled when an upstream proxy URL is configured, Ntlm when Windows auth is on.
type CaptureUpstreamStateDto struct {
	Enabled bool `json:"enabled"`
	Ntlm    bool `json:"ntlm"`
}

// CaptureAccessStateDto reports the proxy access-control mode
// (loopback/lan/allowlist/open) for the status bar.
type CaptureAccessStateDto struct {
	Mode string `json:"mode"`
}

type CertificatesInfosDto struct {
	Available          bool   `json:"available"`
	CaCertSubject      string `json:"ca_cert_subject"`
	CaCertIssuer       string `json:"ca_cert_issuer"`
	CaCertSerialNumber string `json:"ca_cert_serial_number"`
	FingerprintSha256  string `json:"fingerprint_sha256"`
	NotBefore          string `json:"not_before"`
	NotAfter           string `json:"not_after"`
	Expired            bool   `json:"expired"`
	InstallSupported   bool   `json:"install_supported"`
	Installed          bool   `json:"installed"`
	InstallCheckError  string `json:"install_check_error,omitempty"`
	Error              string `json:"error,omitempty"`
}

type CertificateGenerateRequestDto struct {
	Replace bool `json:"replace"`
}

// duplicate of AppConfig to avoid security issues

type AppConfigDto struct {
	Proxy        ProxyConfigDto        `json:"proxy"`
	WebUi        WebUiConfigDto        `json:"web_ui"`
	DecryptHttps DecryptHttpsConfigDto `json:"decrypt_https"`
}

type ProxyConfigDto struct {
	Port                   int                    `json:"port"`
	EnableRemoteConnection bool                   `json:"enable_remote_connection"`
	AccessControl          AccessControlConfigDto `json:"access_control"`
}

type WebUiConfigDto struct {
	Port                   int                    `json:"port"`
	EnableRemoteConnection bool                   `json:"enable_remote_connection"`
	AccessControl          AccessControlConfigDto `json:"access_control"`
}

type DecryptHttpsConfigDto struct {
	Enabled         bool                 `json:"enabled"`
	DefaultMaxBytes *int64               `json:"default_max_bytes,omitempty"`
	MimeTypes       []MimeTypeRuleDto    `json:"mime_types"`
	CertManager     CertManagerConfigDto `json:"cert_manager"`
}

type CertManagerConfigDto struct {
	CaCertFile        string `json:"ca_cert_file"`
	CaKeyFile         string `json:"ca_key_file"`
	DomainCertsFolder string `json:"domain_certs_folder"`
}

type BodyCaptureSettingsDto struct {
	DefaultMaxBytes *int64            `json:"default_max_bytes,omitempty"`
	MimeTypes       []MimeTypeRuleDto `json:"mime_types"`
}

type DecryptHttpsToggleSettingsDto struct {
	Enabled bool `json:"enabled"`
}

// UpstreamSettingsDto is the read/write contract for the outbound (corporate)
// proxy exposed by GET/PUT /api/settings/upstream (B5.2). Changes are persisted
// back to config.yaml; hot re-injection into the running pipeline is out of
// scope for now and takes effect on the next application start.
type UpstreamSettingsDto struct {
	// OutputProxyUri is the upstream proxy URL (e.g. "http://proxy:8080"). Empty
	// means direct connections with no upstream proxy.
	OutputProxyUri string `json:"output_proxy_uri"`
	// NoProxy lists hosts/suffixes that bypass the upstream proxy and connect
	// directly.
	NoProxy []string `json:"no_proxy"`
	// AddWindowsAuthentication enables Windows (NTLM/Negotiate) authentication
	// against the upstream proxy. Only effective on Windows.
	AddWindowsAuthentication bool `json:"add_windows_authentication"`
}

type AccessControlSettingsDto struct {
	Proxy AccessControlConfigDto `json:"proxy"`
	WebUi AccessControlConfigDto `json:"web_ui"`
}

type AccessControlConfigDto struct {
	Mode     string   `json:"mode"`
	Networks []string `json:"networks"`
}

type MimeTypeRuleDto struct {
	Name         string   `json:"name"`
	MaxSizeBytes *int64   `json:"max_size_bytes,omitempty"`
	MaxSizeKb    *float64 `json:"max_size_kb,omitempty"`
	MaxSizeMb    *float64 `json:"max_size_mb,omitempty"`
}
