package middlewares

import (
	"fmt"
	"httpStackLens/http"
	"httpStackLens/http/models"
	"io"
	"net/http/httputil"
	"strconv"
	"strings"
)

func drainUpstreamAuthBody(stream *http.NetworkStream, head models.HttpResponseHead, method models.HttpMethod) error {
	if method == models.HEAD {
		return nil
	}
	if headerHasToken(head.GetHeader("Transfer-Encoding"), "chunked") {
		if _, err := io.Copy(io.Discard, httputil.NewChunkedReader(stream.BufferedReader())); err != nil {
			return err
		}
		// The chunked reader stops before trailers and their final blank line.
		// Consume both before parsing the next authentication response.
		for {
			line, err := stream.ReadLine()
			if err != nil {
				return err
			}
			if line == "" {
				return nil
			}
		}
	}
	lengths := head.GetHeader("Content-Length")
	if len(lengths) != 1 {
		return fmt.Errorf("authentication response has no unambiguous body length")
	}
	length, err := strconv.ParseInt(strings.TrimSpace(lengths[0]), 10, 64)
	if err != nil || length < 0 {
		return fmt.Errorf("invalid authentication response Content-Length")
	}
	_, err = io.CopyN(io.Discard, stream, length)
	return err
}

func upstreamAuthRequest(request models.ProxyRequest) models.ProxyRequest {
	// The client's connection policy is independent of the upstream connection,
	// which must survive all NTLM exchanges. Do not mutate the captured request.
	request.Headers = append([]models.Header(nil), request.Headers...)
	for _, name := range []string{"Connection", "Proxy-Connection"} {
		var tokens []string
		for _, value := range request.GetHeader(name) {
			for _, token := range strings.Split(value, ",") {
				token = strings.TrimSpace(token)
				if token != "" && !strings.EqualFold(token, "close") && !strings.EqualFold(token, "keep-alive") {
					tokens = append(tokens, token)
				}
			}
		}
		tokens = append(tokens, "keep-alive")
		setUpstreamAuthHeader(&request, name, strings.Join(tokens, ", "))
	}
	return request
}

func setUpstreamAuthHeader(request *models.ProxyRequest, name, value string) {
	headers := request.Headers[:0]
	for _, header := range request.Headers {
		if !strings.EqualFold(header.Name, name) {
			headers = append(headers, header)
		}
	}
	request.Headers = append(headers, models.Header{Name: name, Value: value})
}

func headerHasToken(values []string, token string) bool {
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(part), token) {
				return true
			}
		}
	}
	return false
}

func upstreamAuthResponseClosesConnection(head models.HttpResponseHead, method models.HttpMethod) bool {
	connection := head.GetHeader("Connection")
	proxyConnection := head.GetHeader("Proxy-Connection")
	if headerHasToken(connection, "close") || headerHasToken(proxyConnection, "close") {
		return true
	}
	if head.HttpVersion.Major == 1 && head.HttpVersion.Minor == 0 &&
		!headerHasToken(connection, "keep-alive") && !headerHasToken(proxyConnection, "keep-alive") {
		return true
	}
	// A challenge body without length or chunked framing ends at EOF. Its
	// connection cannot carry the next authentication exchange. HEAD has no body.
	return method != models.HEAD && len(head.GetHeader("Content-Length")) == 0 &&
		!headerHasToken(head.GetHeader("Transfer-Encoding"), "chunked")
}

func canRestartUpstreamAuthAfterEOF(request models.ProxyRequest) bool {
	// An EOF gives no final status: avoid replaying an operation with side effects
	// or a request body. CONNECT has not exposed a tunnel to the client yet.
	if len(request.GetHeader("Transfer-Encoding")) != 0 {
		return false
	}
	for _, value := range request.GetHeader("Content-Length") {
		if strings.TrimSpace(value) != "0" {
			return false
		}
	}
	switch request.HttpRequestLine.HttpMethod {
	case models.GET, models.HEAD, models.OPTIONS, models.CONNECT:
		return true
	default:
		return false
	}
}
