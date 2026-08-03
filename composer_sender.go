package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"httpStackLens/certManager"
	"httpStackLens/configuration"
	"httpStackLens/proxy/middlewares"
	"httpStackLens/webui/wasm/shared"
)

const (
	// composerTimeout bounds a composer exchange. It is generous on purpose: the
	// composer is used to poke slow corporate endpoints.
	composerTimeout = 60 * time.Second
	// composerMaxBody caps what is sent back to the browser for display.
	composerMaxBody = 5 << 20
)

// composerSender executes a request built in the Web UI composer through the
// proxy pipeline, exactly as a browser configured to use this proxy would: an
// in-memory connection stands in for the socket, and Go's http.Transport speaks
// the proxy protocol over it — absolute-form request for HTTP, CONNECT then TLS
// for HTTPS.
//
// Nothing here touches the TCP listener, so the composer keeps working while the
// proxy is stopped, still honouring the upstream proxy, no_proxy and HTTPS
// decryption settings currently in effect.
type composerSender struct {
	server  atomic.Pointer[ProxyServer]
	config  func() configuration.AppConfig
	decrypt *configuration.DecryptHttpsConfigStore
	// trustedRoots and maxBody are fields so tests can pin them to a certificate
	// pool and a small limit; newComposerSender installs the real defaults.
	trustedRoots func() *x509.CertPool
	maxBody      int64
}

func newComposerSender(config func() configuration.AppConfig, decrypt *configuration.DecryptHttpsConfigStore) *composerSender {
	sender := &composerSender{config: config, decrypt: decrypt, maxBody: composerMaxBody}
	sender.trustedRoots = sender.rootCAs
	return sender
}

// attach publishes the pipeline entry point. The Web UI server is started
// before the event logger and the stores exist, so the sender is handed to it
// empty and completed here.
func (s *composerSender) attach(server *ProxyServer) {
	s.server.Store(server)
}

func (s *composerSender) Send(ctx context.Context, dto shared.ComposerRequestDto) (shared.ComposerResponseDto, error) {
	server := s.server.Load()
	if server == nil {
		return shared.ComposerResponseDto{}, errors.New("the proxy pipeline is not ready yet")
	}

	request, err := s.buildRequest(ctx, dto)
	if err != nil {
		return shared.ComposerResponseDto{}, err
	}

	client := &http.Client{
		Transport: s.transport(server),
		Timeout:   composerTimeout,
		// The composer reports what the server answered, redirects included.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}

	started := time.Now()
	response, err := client.Do(request)
	elapsed := int(time.Since(started).Milliseconds())
	if err != nil {
		// A request that never got a response is a result to display, not an API
		// failure: the composer shows the transport error where the status goes.
		return shared.ComposerResponseDto{DurationMs: elapsed, Upstream: s.upstream(), Error: transportError(err)}, nil
	}
	defer func() { _ = response.Body.Close() }()

	body, truncated, err := readLimited(response.Body, s.maxBody)
	if err != nil {
		return shared.ComposerResponseDto{
			DurationMs: int(time.Since(started).Milliseconds()),
			Status:     response.StatusCode,
			StatusText: statusText(response),
			Headers:    headerDtos(response.Header),
			Upstream:   s.upstream(),
			Error:      fmt.Sprintf("could not read the response body: %v", err),
		}, nil
	}

	return shared.ComposerResponseDto{
		Status:     response.StatusCode,
		StatusText: statusText(response),
		Headers:    headerDtos(response.Header),
		Body:       string(body),
		DurationMs: int(time.Since(started).Milliseconds()),
		Truncated:  truncated,
		Upstream:   s.upstream(),
	}, nil
}

// buildRequest validates the composer input. Errors here are the caller's
// mistakes (bad URL, bad method) and surface as 400s.
func (s *composerSender) buildRequest(ctx context.Context, dto shared.ComposerRequestDto) (*http.Request, error) {
	method := strings.ToUpper(strings.TrimSpace(dto.Method))
	if method == "" {
		method = http.MethodGet
	}

	target, err := url.Parse(strings.TrimSpace(dto.Url))
	if err != nil {
		return nil, fmt.Errorf("invalid URL %q: %w", dto.Url, err)
	}
	switch strings.ToLower(target.Scheme) {
	case "http", "https":
	case "":
		return nil, fmt.Errorf("URL %q must start with http:// or https://", dto.Url)
	default:
		return nil, fmt.Errorf("unsupported scheme %q: the composer sends HTTP and HTTPS", target.Scheme)
	}
	if target.Host == "" {
		return nil, fmt.Errorf("URL %q has no host", dto.Url)
	}

	var body io.Reader
	if dto.Body != "" {
		body = strings.NewReader(dto.Body)
	}
	request, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	for _, header := range dto.Headers {
		name := strings.TrimSpace(header.Name)
		if name == "" {
			continue
		}
		// Host is not a regular header: net/http reads it from the request.
		if strings.EqualFold(name, "Host") {
			request.Host = header.Value
			continue
		}
		request.Header.Add(name, header.Value)
	}
	return request, nil
}

// transport turns every dial into an in-memory connection served by the
// pipeline. The proxy URL is never contacted — it only tells the transport to
// speak the proxy protocol, as a browser pointed at HttpStackLens would.
func (s *composerSender) transport(server *ProxyServer) *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyURL(&url.URL{Scheme: "http", Host: "pipeline.internal"}),
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			client, pipeline := net.Pipe()
			// Marked local so the browser-authentication layer lets it through:
			// there is no external client to challenge here.
			go server.ServeConnection(middlewares.LocalConn{Conn: pipeline})
			return client, nil
		},
		TLSClientConfig: &tls.Config{RootCAs: s.trustedRoots()},
		// One request per connection keeps the pipeline entry point simple: it
		// reads exactly one proxy request per connection, as it does for a
		// freshly accepted client.
		DisableKeepAlives:   true,
		TLSHandshakeTimeout: composerTimeout,
	}
}

// rootCAs trusts the system roots, plus this application's CA while HTTPS
// decryption is on — the interceptor then answers with a certificate it signed
// itself, exactly as it does for a browser.
func (s *composerSender) rootCAs() *x509.CertPool {
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if s.decrypt == nil || !s.decrypt.Get().Enabled || s.config == nil {
		return pool
	}

	config := s.config()
	certConfig := config.DecryptHttps.CertManager
	if certConfig.CaCertFile == "" || certConfig.CaKeyFile == "" {
		return pool
	}
	caCert, _, err := certManager.GetHttpsDebugRootCertificates(config)
	if err != nil || caCert == nil {
		return pool
	}
	pool.AddCert(caCert)
	return pool
}

// upstream reports the outbound proxy in effect, so the UI can tell a direct
// connection from one that went through the corporate gateway.
func (s *composerSender) upstream() string {
	if s.config == nil {
		return ""
	}
	return s.config().Proxy.OutputProxyUri
}

func statusText(response *http.Response) string {
	// Go keeps the reason phrase in Status as "404 Not Found".
	if _, text, found := strings.Cut(response.Status, " "); found {
		return text
	}
	return response.Status
}

func headerDtos(header http.Header) []shared.HeaderDto {
	dtos := make([]shared.HeaderDto, 0, len(header))
	for name, values := range header {
		for _, value := range values {
			dtos = append(dtos, shared.HeaderDto{Name: name, Value: value})
		}
	}
	return dtos
}

func readLimited(reader io.Reader, limit int64) ([]byte, bool, error) {
	body, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(body)) > limit {
		return body[:limit], true, nil
	}
	return body, false, nil
}

// transportError unwraps the layers net/http adds around a dial or TLS failure,
// which would otherwise show the fake proxy host to the user.
func transportError(err error) string {
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Err != nil {
		return urlErr.Err.Error()
	}
	return err.Error()
}
