package middlewares

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"fmt"
	"httpStackLens/certManager"
	"httpStackLens/configuration"
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
	// Limits drives the per-content-type body size caps. Bodies larger than the
	// limit are forwarded to the browser but not stored (BodySkipped is set).
	Limits configuration.CaptureConfig
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
	//
	// DisableCompression keeps the transport from silently adding
	// "Accept-Encoding: gzip" (which it does whenever the request carries none)
	// and transparently decompressing the reply. That transparent path drops the
	// response's Content-Length, which would force close-delimited framing and
	// stall keep-alive. Combined with forward() deleting Accept-Encoding, the
	// origin returns an identity body we can stream, frame and store as-is.
	transport := &http.Transport{
		TLSClientConfig:    &tls.Config{NextProtos: []string{"http/1.1"}},
		ForceAttemptHTTP2:  false,
		DisableCompression: true,
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

	// One id correlates the request record with its response record. The request
	// body is captured within its size limit while still being forwarded in full.
	recordID, _ := storage.NewUUID()
	reqBody, reqSkipped, err := m.capRequestBody(req)
	if err != nil {
		return err
	}
	m.recordRequest(recordID, req, reqBody, reqSkipped)

	log.Printf("🔓 %s https://%s%s\n", req.Method, host, req.URL.RequestURI())

	resp, err := transport.RoundTrip(req)
	if err != nil {
		return err
	}
	originalBody := resp.Body
	defer originalBody.Close()

	contentType := resp.Header.Get("Content-Type")
	limit, _ := m.Limits.LimitForContentType(contentType)

	// Stream the response to the browser as it arrives, capturing at most `limit`
	// bytes in parallel through a tee. The response keeps its original framing
	// (Content-Length / chunked), so the browser always sees a correctly
	// delimited message.
	//
	// This must not buffer the whole body before forwarding: doing so used to
	// stall progressive and long-lived responses — video segments, SSE,
	// long-poll — until they closed, leaving the browser stuck in "loading".
	var capture *captureLimitWriter
	if m.Capture != nil || isHtmlOrJs(contentType) {
		capture = &captureLimitWriter{limit: limit}
		resp.Body = io.NopCloser(io.TeeReader(originalBody, capture))
	}

	// A body of unknown length with no chunked framing would be delimited by
	// connection close, but the tunnel is kept alive for the next request, so the
	// browser would wait forever. Force chunked framing so every response carries
	// an explicit terminator. (0-or-positive lengths and HEAD/204/304 replies,
	// which have no body, are left untouched.)
	if resp.ContentLength < 0 && len(resp.TransferEncoding) == 0 {
		resp.TransferEncoding = []string{"chunked"}
	}

	writeErr := resp.Write(clientTLS)

	if capture != nil {
		body, skipped := capture.captured()
		if !skipped {
			printIfTextual(host, req.URL.RequestURI(), contentType, body)
		}
		m.recordResponse(recordID, resp, body, skipped)
	}

	return writeErr
}

// captureLimitWriter accumulates the bytes written to it up to limit, so a
// response body can be captured for storage while it is being streamed to the
// browser via io.TeeReader. Writes always report the full length, so they never
// throttle the stream. Once the total exceeds limit the buffered bytes are
// dropped and skipped is latched: the body is still forwarded in full, but not
// stored (mirroring the previous BodySkipped semantics).
type captureLimitWriter struct {
	limit   int64
	written int64
	buf     bytes.Buffer
	skipped bool
}

func (w *captureLimitWriter) Write(p []byte) (int, error) {
	w.written += int64(len(p))
	if !w.skipped {
		if w.written > w.limit {
			w.skipped = true
			w.buf.Reset()
		} else {
			w.buf.Write(p)
		}
	}
	return len(p), nil
}

// captured returns the buffered body and whether it was skipped for exceeding
// the limit. When skipped, the body is nil (nothing is stored).
func (w *captureLimitWriter) captured() (body []byte, skipped bool) {
	if w.skipped {
		return nil, true
	}
	return w.buf.Bytes(), false
}

// capBody reads up to limit+1 bytes from body to decide whether it fits.
//
//   - It always returns a forward reader that replays the *entire* body, so the
//     caller can forward it in full regardless of the limit. io.LimitReader only
//     bounds how much is read; the rest stays in body and is chained back with
//     io.MultiReader.
//   - When the body fits (<= limit), store holds the buffered bytes and
//     skipped is false. When it exceeds the limit, store is nil and skipped is
//     true (the body is forwarded but not captured).
func capBody(body io.Reader, limit int64) (store []byte, forward io.Reader, skipped bool, err error) {
	head, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, nil, false, err
	}
	if int64(len(head)) > limit {
		return nil, io.MultiReader(bytes.NewReader(head), body), true, nil
	}
	return head, bytes.NewReader(head), false, nil
}

// capRequestBody applies capBody to the request body, re-attaching a full
// replay for forwarding upstream. It is a no-op when capture is off or the
// request has no body.
func (m *HttpsInterceptor) capRequestBody(req *http.Request) (store []byte, skipped bool, err error) {
	if m.Capture == nil || req.Body == nil {
		return nil, false, nil
	}
	limit, _ := m.Limits.LimitForContentType(req.Header.Get("Content-Type"))

	store, forward, skipped, err := capBody(req.Body, limit)
	if err != nil {
		return nil, false, err
	}
	req.Body = io.NopCloser(forward)
	if !skipped {
		req.ContentLength = int64(len(store))
	}
	return store, skipped, nil
}

func (m *HttpsInterceptor) recordRequest(id storage.UUID, req *http.Request, body []byte, skipped bool) {
	if m.Capture == nil {
		return
	}
	rec := storage.RequestRecord{
		RequestID:   id,
		Method:      req.Method,
		URL:         req.URL.String(),
		HttpVersion: storage.NewHttpVersion(req.ProtoMajor, req.ProtoMinor),
		Headers:     httpHeadersToRecords(req.Header),
		BodySkipped: skipped,
		Body:        body,
	}
	if err := m.Capture.WriteRequest(rec); err != nil {
		log.Printf("⚠️  capture: failed to record request: %v\n", err)
	}
}

func (m *HttpsInterceptor) recordResponse(id storage.UUID, resp *http.Response, body []byte, skipped bool) {
	if m.Capture == nil {
		return
	}
	rec := storage.ResponseRecord{
		RequestID:     id,
		HttpVersion:   storage.NewHttpVersion(resp.ProtoMajor, resp.ProtoMinor),
		StatusCode:    int16(resp.StatusCode),
		StatusMessage: statusMessage(resp),
		Headers:       httpHeadersToRecords(resp.Header),
		BodySkipped:   skipped,
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
