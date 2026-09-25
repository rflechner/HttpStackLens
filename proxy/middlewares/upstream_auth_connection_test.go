package middlewares

import (
	"httpStackLens/http"
	"httpStackLens/http/models"
	"net"
	"reflect"
	"testing"
	"time"
)

func TestUpstreamAuthRequestKeepsConnectionAliveWithoutMutatingClientRequest(t *testing.T) {
	headers := []models.Header{
		{Name: "Connection", Value: "Upgrade, close"},
		{Name: "connection", Value: "CLOSE"},
		{Name: "Proxy-Connection", Value: "close"},
		{Name: "Authorization", Value: "Bearer origin-token"},
	}
	request := models.ProxyRequest{Headers: append([]models.Header(nil), headers...)}
	upstream := upstreamAuthRequest(request)
	if got := upstream.GetHeader("Connection"); !reflect.DeepEqual(got, []string{"Upgrade, keep-alive"}) {
		t.Fatalf("Connection = %v", got)
	}
	if got := upstream.GetHeader("Proxy-Connection"); !reflect.DeepEqual(got, []string{"keep-alive"}) {
		t.Fatalf("Proxy-Connection = %v", got)
	}
	setUpstreamAuthHeader(&upstream, "Authorization", "NTLM test-token")
	if !reflect.DeepEqual(request.Headers, headers) {
		t.Fatalf("client request mutated: %+v", request.Headers)
	}
}

func TestUpstreamAuthDetectsNonReusableConnections(t *testing.T) {
	cases := []struct {
		name    string
		version models.Version
		headers []models.Header
		method  models.HttpMethod
		close   bool
	}{
		{"HTTP/1.1 framed", models.Version{1, 1}, []models.Header{{Name: "Content-Length", Value: "0"}}, models.GET, false},
		{"explicit close", models.Version{1, 1}, []models.Header{{Name: "Connection", Value: "keep-alive, CLOSE"}, {Name: "Content-Length", Value: "0"}}, models.GET, true},
		{"proxy close", models.Version{1, 1}, []models.Header{{Name: "Proxy-Connection", Value: "close"}, {Name: "Content-Length", Value: "0"}}, models.GET, true},
		{"HTTP/1.0", models.Version{1, 0}, []models.Header{{Name: "Content-Length", Value: "0"}}, models.GET, true},
		{"HTTP/1.0 keep-alive", models.Version{1, 0}, []models.Header{{Name: "Connection", Value: "keep-alive"}, {Name: "Content-Length", Value: "0"}}, models.GET, false},
		{"EOF body", models.Version{1, 1}, nil, models.GET, true},
		{"chunked body", models.Version{1, 1}, []models.Header{{Name: "Transfer-Encoding", Value: "chunked"}}, models.GET, false},
		{"HEAD without body", models.Version{1, 1}, nil, models.HEAD, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			head := models.HttpResponseHead{HttpVersion: tc.version, StatusCode: 407, Headers: tc.headers}
			if got := upstreamAuthResponseClosesConnection(head, tc.method); got != tc.close {
				t.Fatalf("connection closes = %v, want %v", got, tc.close)
			}
		})
	}
}

func TestUpstreamAuthDoesNotReplayUnsafeRequestsAfterEOF(t *testing.T) {
	for _, method := range []models.HttpMethod{models.POST, models.PUT, models.PATCH, models.DELETE} {
		if canRestartUpstreamAuthAfterEOF(models.ProxyRequest{HttpRequestLine: models.HttpRequestLine{HttpMethod: method}}) {
			t.Fatalf("must not replay %s after an ambiguous EOF", method)
		}
	}
	for _, headers := range [][]models.Header{
		{{Name: "Content-Length", Value: "1"}},
		{{Name: "Transfer-Encoding", Value: "chunked"}},
	} {
		if canRestartUpstreamAuthAfterEOF(models.ProxyRequest{HttpRequestLine: models.HttpRequestLine{HttpMethod: models.GET}, Headers: headers}) {
			t.Fatal("must not replay a request body")
		}
	}
}

func TestDrainUpstreamAuthChunkedBodyConsumesTrailers(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(5 * time.Second))
	go func() {
		defer server.Close()
		_, _ = server.Write([]byte("4\r\nbody\r\n0\r\nX-Trailer: value\r\n\r\nHTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n"))
	}()
	stream := http.NewNetworkStream(client)
	head := models.HttpResponseHead{Headers: []models.Header{{Name: "Transfer-Encoding", Value: "chunked"}}}
	if err := drainUpstreamAuthBody(stream, head, models.GET); err != nil {
		t.Fatal(err)
	}
	response, err := http.ReadHttpResponse(stream)
	if err != nil || response.StatusCode != 200 {
		t.Fatalf("next response = %+v, %v", response, err)
	}
}
