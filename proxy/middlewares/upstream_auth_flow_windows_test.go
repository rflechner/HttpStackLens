package middlewares

import (
	"bufio"
	"encoding/base64"
	"fmt"
	lenshttp "httpStackLens/http"
	"httpStackLens/http/models"
	"httpStackLens/security"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// Scripted tokens exercise the real middleware without using Windows credentials.
// Type 3 embeds the connection's challenge so stale contexts/tokens are detectable.
type scriptedClientAuth struct {
	started  bool
	released int
}

func (a *scriptedClientAuth) Update(token []byte) (bool, []byte, error) {
	if !a.started {
		if len(token) != 0 {
			return false, nil, fmt.Errorf("new context received a challenge from a previous connection")
		}
		a.started = true
		return false, []byte("type-1"), nil
	}
	if !strings.HasPrefix(string(token), "challenge-") {
		return false, nil, fmt.Errorf("expected a server challenge")
	}
	return true, []byte("type-3:" + string(token)), nil
}

func (a *scriptedClientAuth) Release() error {
	a.released++
	return nil
}

type authExchange struct {
	token string
	reply string // An empty reply closes the socket without a response status.
}

func authReply(status int, token, extra string) string {
	header := "Proxy-Authenticate"
	if status == 401 {
		header = "WWW-Authenticate"
	}
	value := "NTLM"
	if token != "" {
		value += " " + base64.StdEncoding.EncodeToString([]byte(token))
	}
	return fmt.Sprintf("HTTP/1.1 %d Authentication required\r\n%s: %s\r\nContent-Length: 0\r\n%s\r\n", status, header, value, extra)
}

func TestUpstreamAuthenticationConnectionLifecycle(t *testing.T) {
	ok := "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nOK"
	bare := authReply(407, "", "")
	type2 := authReply(407, "challenge-new", "")
	finish := []authExchange{{"type-1", type2}, {"type-3:challenge-new", ok}}
	cases := []struct {
		name     string
		method   models.HttpMethod
		status   int
		scripts  [][]authExchange
		wantErr  string
		wantBody string
		contexts int
	}{
		{"client close headers", models.GET, 200, [][]authExchange{append([]authExchange{{"", bare}}, finish...)}, "", "OK", 1},
		{"initial challenge closes", models.GET, 200, [][]authExchange{{{"", authReply(407, "", "Connection: close\r\n")}}, finish}, "", "OK", 1},
		{"silent EOF after Type 1", models.GET, 200, [][]authExchange{{{"", bare}, {"type-1", ""}}, finish}, "", "OK", 2},
		{"CONNECT silent EOF", models.CONNECT, 200, [][]authExchange{{{"", bare}, {"type-1", ""}}, finish}, "", "OK", 2},
		{"Type 2 closes", models.GET, 200, [][]authExchange{{{"", bare}, {"type-1", authReply(407, "challenge-old", "Connection: close\r\n")}}, finish}, "", "OK", 2},
		{"proxy connection closes", models.GET, 200, [][]authExchange{{{"", authReply(407, "", "Proxy-Connection: close\r\n")}}, finish}, "", "OK", 1},
		{"HTTP/1.0 closes", models.GET, 200, [][]authExchange{{{"", strings.Replace(bare, "HTTP/1.1", "HTTP/1.0", 1)}}, finish}, "", "OK", 1},
		{"EOF delimited challenge", models.GET, 200, [][]authExchange{{{"", strings.Replace(bare, "Content-Length: 0\r\n", "", 1) + "challenge body"}}, finish}, "", "OK", 1},
		{"401 compatibility", models.GET, 200, [][]authExchange{{{"", authReply(401, "", "Connection: close\r\n")}}, {{"type-1", authReply(401, "challenge-new", "")}, {"type-3:challenge-new", ok}}}, "", "OK", 1},
		{"chunked challenge trailers", models.GET, 200, [][]authExchange{append([]authExchange{{"", strings.Replace(bare, "Content-Length: 0\r\n", "Transfer-Encoding: chunked\r\n", 1) + "4\r\nbody\r\n0\r\nX-Trailer: value\r\n\r\n"}}, finish...)}, "", "OK", 1},
		{"bounded reconnects", models.GET, 0, [][]authExchange{{{"", bare}, {"type-1", ""}}, {{"type-1", ""}}, {{"type-1", ""}}}, "2 reconnects exhausted", "", 3},
		{"POST is not replayed after EOF", models.POST, 0, [][]authExchange{{{"", bare}, {"type-1", ""}}}, "EOF", "", 1},
		{"no replay after Type 3", models.GET, 0, [][]authExchange{{{"", bare}, {"type-1", type2}, {"type-3:challenge-new", ""}}}, "EOF", "", 1},
		{"unsupported challenge keeps body", models.GET, 407, [][]authExchange{{{"", "HTTP/1.1 407 Authentication required\r\nProxy-Authenticate: Basic realm=test\r\nContent-Length: 6\r\n\r\ndenied"}}}, "", "denied", 0},
		{"rejected Type 3 keeps body", models.GET, 407, [][]authExchange{{{"", bare}, {"type-1", type2}, {"type-3:challenge-new", "HTTP/1.1 407 Authentication required\r\nProxy-Authenticate: NTLM\r\nContent-Length: 6\r\n\r\ndenied"}}}, "", "denied", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			_ = listener.(*net.TCPListener).SetDeadline(time.Now().Add(10 * time.Second))
			authHeader := "Proxy-Authorization"
			compatibility := tc.name == "401 compatibility"
			if compatibility {
				authHeader = "Authorization"
			}
			serverDone := make(chan error, 1)
			go func() {
				for index, script := range tc.scripts {
					conn, err := listener.Accept()
					if err != nil {
						serverDone <- fmt.Errorf("connection %d: %w", index, err)
						return
					}
					err = serveAuthScript(conn, script, authHeader, tc.method)
					_ = conn.Close()
					if err != nil {
						serverDone <- fmt.Errorf("connection %d: %w", index, err)
						return
					}
				}
				serverDone <- nil
			}()
			proxyURL, _ := url.Parse("http://" + listener.Addr().String())
			var contexts []*scriptedClientAuth
			middleware := ForwardProxyServerWithWindowsAuthentication{
				Forwarder:                     ForwardProxyServer{OutputProxy: *proxyURL},
				Treat401AsProxyAuthentication: compatibility,
				newClientAuth: func(pkg security.AuthPackage) (upstreamClientAuth, error) {
					if pkg != security.AuthNTLM {
						return nil, fmt.Errorf("unexpected auth package")
					}
					auth := &scriptedClientAuth{}
					contexts = append(contexts, auth)
					return auth, nil
				},
			}
			request := models.ProxyRequest{
				HttpRequestLine: models.HttpRequestLine{HttpMethod: tc.method, Version: models.Version{Major: 1, Minor: 1},
					Endpoint: models.ResourceEndpoint{Scheme: "http", Host: "archive.ubuntu.com", Port: 80, PathAndQuery: "/ubuntu/dists/noble-updates/InRelease"}},
				Headers: []models.Header{{Name: "Host", Value: "archive.ubuntu.com"}, {Name: "Connection", Value: "close"}, {Name: "Proxy-Connection", Value: "close"}},
			}
			browser, client := net.Pipe()
			defer client.Close()
			_ = client.SetDeadline(time.Now().Add(10 * time.Second))
			result := make(chan error, 1)
			go func() {
				defer browser.Close()
				result <- middleware.HandleProxyRequest(browser, request)
			}()
			response, responseErr := http.ReadResponse(bufio.NewReader(client), &http.Request{Method: string(tc.method)})
			if tc.status != 0 {
				if responseErr != nil {
					t.Fatalf("client response: %v", responseErr)
				}
				body, err := io.ReadAll(response.Body)
				response.Body.Close()
				if err != nil || response.StatusCode != tc.status || string(body) != tc.wantBody {
					t.Fatalf("response = %d %q, err %v", response.StatusCode, body, err)
				}
			} else if responseErr == nil {
				t.Fatal("expected the middleware to report a connection failure")
			}
			_ = client.Close()
			err = <-result
			if tc.wantErr == "" && err != nil || tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("middleware error = %v, want %q", err, tc.wantErr)
			}
			if err := <-serverDone; err != nil {
				t.Fatal(err)
			}
			if len(contexts) != tc.contexts {
				t.Fatalf("created %d contexts, want %d", len(contexts), tc.contexts)
			}
			for _, auth := range contexts {
				if auth.released != 1 {
					t.Fatalf("context released %d times", auth.released)
				}
			}
		})
	}
}

func serveAuthScript(conn net.Conn, script []authExchange, authHeader string, method models.HttpMethod) error {
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	stream := lenshttp.NewNetworkStream(conn)
	for _, exchange := range script {
		request, err := lenshttp.ReadProxyRequest(stream)
		if err != nil {
			return err
		}
		if request.HttpRequestLine.HttpMethod != method || request.HttpRequestLine.Endpoint.Host != "archive.ubuntu.com" {
			return fmt.Errorf("unexpected request target")
		}
		for _, name := range []string{"Connection", "Proxy-Connection"} {
			values := request.GetHeader(name)
			if len(values) != 1 || values[0] != "keep-alive" {
				return fmt.Errorf("%s must request keep-alive, got %v", name, values)
			}
		}
		wantAuth := ""
		if exchange.token != "" {
			wantAuth = "NTLM " + base64.StdEncoding.EncodeToString([]byte(exchange.token))
		}
		values := request.GetHeader(authHeader)
		if wantAuth == "" && len(values) != 0 || wantAuth != "" && (len(values) != 1 || values[0] != wantAuth) {
			return fmt.Errorf("unexpected authentication stage")
		}
		if exchange.reply == "" {
			return nil
		}
		if _, err := io.WriteString(conn, exchange.reply); err != nil {
			return err
		}
	}
	return nil
}
