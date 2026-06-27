package middlewares

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"fmt"
	"httpStackLens/certManager"
	"httpStackLens/http/models"
	"httpStackLens/storage"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
)

// HttpsInterceptor performs a man-in-the-middle on CONNECT tunnels so that the
// HTTPS traffic can be inspected in clear text: it terminates the browser's TLS
// session with a certificate minted for the target host, opens its own TLS
// session to the real server, and relays HTTP requests/responses in between.
//
// Any non-CONNECT request (or when no CertStore is configured) is handed over
// to the next middleware untouched.
type HttpsInterceptor struct {
	CertStore *certManager.CertStore
	Next      Middleware
	// Capture, when non-nil, receives the decrypted requests and responses so
	// they are persisted in clear text to the capture file.
	Capture storage.CaptureSessionWriter
}

func (m *HttpsInterceptor) HandleProxyRequest(browser net.Conn, request models.ProxyRequest) error {
	if m.CertStore == nil || !request.HttpRequestLine.IsConnect() {
		return m.Next.HandleProxyRequest(browser, request)
	}
	return m.intercept(browser, request)
}

func (m *HttpsInterceptor) intercept(browser net.Conn, request models.ProxyRequest) error {
	host := request.HttpRequestLine.Endpoint.Host
	authority := net.JoinHostPort(host, strconv.Itoa(request.HttpRequestLine.Endpoint.Port))

	// Tell the browser the tunnel is ready, then take over the TLS session
	// instead of blindly piping bytes.
	if _, err := browser.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
		return err
	}

	clientTLS := tls.Server(browser, &tls.Config{
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			name := hello.ServerName
			if name == "" {
				name = host // older clients may not send SNI
			}
			return m.CertStore.GetCertificate(name)
		},
	})
	defer clientTLS.Close()

	if err := clientTLS.Handshake(); err != nil {
		log.Printf("🔓 TLS handshake with browser failed for %s: %v\n", host, err)
		return err
	}

	// One transport per tunnel, reused across keep-alive requests. It dials the
	// real server over TLS and verifies its certificate normally.
	transport := &http.Transport{
		TLSClientConfig:   &tls.Config{NextProtos: []string{"http/1.1"}},
		ForceAttemptHTTP2: false,
	}
	defer transport.CloseIdleConnections()

	reader := bufio.NewReader(clientTLS)
	for {
		req, err := http.ReadRequest(reader)
		if err != nil {
			if err != io.EOF {
				log.Printf("🔓 Could not read decrypted request for %s: %v\n", host, err)
			}
			return nil
		}

		closeConn := req.Close
		if err := m.forward(clientTLS, transport, req, host, authority); err != nil {
			log.Printf("🔓 Error proxying decrypted request to %s: %v\n", host, err)
			return nil
		}
		if closeConn {
			return nil
		}
	}
}

// forward sends one decrypted request to the real server and writes the
// response back to the browser, printing the body when it is HTML or JS.
func (m *HttpsInterceptor) forward(clientTLS net.Conn, transport *http.Transport, req *http.Request, host, authority string) error {
	// ReadRequest yields a server-style request (path only); turn it into an
	// absolute one the transport can send. Dropping Accept-Encoding asks the
	// origin for an uncompressed body so we can print/store it as-is.
	req.URL.Scheme = "https"
	req.URL.Host = authority
	req.RequestURI = ""
	req.Header.Del("Accept-Encoding")

	// One id correlates the request record with its response record. Buffer the
	// request body so we can both capture it and still forward it upstream.
	recordID, _ := storage.NewUUID()
	reqBody, err := m.bufferRequestBody(req)
	if err != nil {
		return err
	}
	m.recordRequest(recordID, req, reqBody)

	log.Printf("🔓 %s https://%s%s\n", req.Method, host, req.URL.RequestURI())

	resp, err := transport.RoundTrip(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	printIfTextual(host, req.URL.RequestURI(), resp.Header.Get("Content-Type"), body)
	m.recordResponse(recordID, resp, body)

	// Re-attach the buffered body and force a Content-Length framing so the
	// response can be written back to the browser.
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	resp.TransferEncoding = nil

	return resp.Write(clientTLS)
}

// bufferRequestBody reads the request body so it can be captured, then restores
// it for forwarding. It is a no-op (returns nil) when capture is off or the
// request has no body.
func (m *HttpsInterceptor) bufferRequestBody(req *http.Request) ([]byte, error) {
	if m.Capture == nil || req.Body == nil {
		return nil, nil
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	_ = req.Body.Close()
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	return body, nil
}

func (m *HttpsInterceptor) recordRequest(id storage.UUID, req *http.Request, body []byte) {
	if m.Capture == nil {
		return
	}
	rec := storage.RequestRecord{
		RequestID:   id,
		Method:      req.Method,
		URL:         req.URL.String(),
		HttpVersion: storage.NewHttpVersion(req.ProtoMajor, req.ProtoMinor),
		Headers:     httpHeadersToRecords(req.Header),
		Body:        nilIfEmpty(body),
	}
	if err := m.Capture.WriteRequest(rec); err != nil {
		log.Printf("⚠️  capture: failed to record request: %v\n", err)
	}
}

func (m *HttpsInterceptor) recordResponse(id storage.UUID, resp *http.Response, body []byte) {
	if m.Capture == nil {
		return
	}
	rec := storage.ResponseRecord{
		RequestID:     id,
		HttpVersion:   storage.NewHttpVersion(resp.ProtoMajor, resp.ProtoMinor),
		StatusCode:    int16(resp.StatusCode),
		StatusMessage: statusMessage(resp),
		Headers:       httpHeadersToRecords(resp.Header),
		Body:          body,
	}
	if err := m.Capture.WriteResponse(rec); err != nil {
		log.Printf("⚠️  capture: failed to record response: %v\n", err)
	}
}

// httpHeadersToRecords flattens an http.Header map into ordered name/value
// pairs (one entry per value, so duplicates survive).
func httpHeadersToRecords(h http.Header) []storage.Header {
	if len(h) == 0 {
		return nil
	}
	out := make([]storage.Header, 0, len(h))
	for name, values := range h {
		for _, v := range values {
			out = append(out, storage.Header{Name: name, Value: v})
		}
	}
	return out
}

// statusMessage extracts the reason phrase from resp.Status ("200 OK" -> "OK"),
// falling back to the canonical text for the status code.
func statusMessage(resp *http.Response) string {
	if _, msg, ok := strings.Cut(resp.Status, " "); ok {
		return msg
	}
	return http.StatusText(resp.StatusCode)
}

func nilIfEmpty(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	return b
}

func printIfTextual(host, path, contentType string, body []byte) {
	if !isHtmlOrJs(contentType) {
		return
	}
	fmt.Printf("\n===== 🔓 Decrypted https://%s%s (%s) =====\n%s\n===== end =====\n\n",
		host, path, contentType, body)
}

func isHtmlOrJs(contentType string) bool {
	ct := strings.ToLower(contentType)
	return strings.Contains(ct, "text/html") ||
		strings.Contains(ct, "javascript") ||
		strings.Contains(ct, "ecmascript")
}
