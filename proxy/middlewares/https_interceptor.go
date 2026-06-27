package middlewares

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"fmt"
	"httpStackLens/certManager"
	"httpStackLens/http/models"
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
	// origin for an uncompressed body so we can print it as-is.
	req.URL.Scheme = "https"
	req.URL.Host = authority
	req.RequestURI = ""
	req.Header.Del("Accept-Encoding")

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

	// Re-attach the buffered body and force a Content-Length framing so the
	// response can be written back to the browser.
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	resp.TransferEncoding = nil

	return resp.Write(clientTLS)
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
