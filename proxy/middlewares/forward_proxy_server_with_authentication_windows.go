package middlewares

import (
	"encoding/base64"
	"fmt"
	"httpStackLens/http"
	"httpStackLens/http/models"
	"httpStackLens/security"
	"io"
	"log/slog"
	"net"
	"strings"
)

type ForwardProxyServerWithWindowsAuthentication struct {
	Forwarder                     ForwardProxyServer
	Treat401AsProxyAuthentication bool
}

type upstreamAuthChallenge struct {
	authenticateHeader  string
	authorizationHeader string
}

func (m *ForwardProxyServerWithWindowsAuthentication) HandleProxyRequest(browser net.Conn, request models.ProxyRequest) error {
	// Contextual logger enriched once, like a Serilog logger with bound
	// properties; every step below inherits the client/target attributes.
	logger := slog.With(
		"component", "upstream-windows-auth",
		"client", browser.RemoteAddr().String(),
		"target", request.HttpRequestLine.String(),
	)
	logger.Debug("handling request with upstream Windows authentication")

	gatewayConnection, err := m.Forwarder.ConnectToGateway(browser, request)
	if err != nil {
		logger.Error("failed to connect to upstream gateway", "error", err)
		return err
	}
	defer gatewayConnection.Close()
	logger.Debug("connected to upstream gateway", "gateway", m.Forwarder.OutputProxy.Host)

	gateway := http.NewNetworkStream(gatewayConnection)

	var clientAuth *security.ClientAuth
	defer func() {
		if clientAuth != nil {
			clientAuth.Release()
		}
	}()

	var authValue string

	currentRequest := request
	attempt := 0
	for {
		attempt++
		logger.Debug("sending request to gateway", "attempt", attempt)

		// Send request to gateway
		_, err = currentRequest.WriteTo(gateway, true)
		if err != nil {
			logger.Error("failed to write request to gateway", "attempt", attempt, "error", err)
			return fmt.Errorf("failed to write request to gateway: %w", err)
		}

		// Read response head from gateway
		responseHead, err := http.ReadHttpResponse(gateway)
		if err != nil {
			logger.Error("failed to read response from gateway", "attempt", attempt, "error", err)
			return fmt.Errorf("failed to read response from gateway: %w", err)
		}
		logger.Debug("received response from gateway", "attempt", attempt, "status", responseHead.StatusCode)

		challenge, ok := m.detectUpstreamAuthChallenge(responseHead)
		if !ok {
			logger.Debug("no auth challenge, forwarding response and tunneling", "status", responseHead.StatusCode)
			_, err = responseHead.WriteTo(browser)
			if err != nil {
				logger.Error("failed to forward response to client", "error", err)
				return err
			}

			go io.Copy(browser, gateway)
			io.Copy(gateway, browser)

			logger.Info("upstream auth flow completed, connection closed", "attempts", attempt, "status", responseHead.StatusCode)
			// normal exit
			return nil
		}
		logger.Debug("upstream auth challenge detected",
			"status", responseHead.StatusCode,
			"authenticateHeader", challenge.authenticateHeader)

		// Read response body from gateway
		_, err = http.ReadHttpResponseBody(gateway, responseHead)
		if err != nil {
			logger.Error("failed to read challenge response body from gateway", "error", err)
			return fmt.Errorf("failed to read response body from gateway: %w", err)
		}

		authHeaders := responseHead.GetHeader(challenge.authenticateHeader)
		if len(authHeaders) == 0 {
			logger.Warn("auth challenge without authenticate header, forwarding as-is",
				"header", challenge.authenticateHeader)
			_, err = responseHead.WriteTo(browser)
			if err != nil {
				return err
			}
			_, err = io.Copy(browser, gateway)
			return err
		}

		// Choose an auth package and token
		var selectedPackage security.AuthPackage = security.AuthNone
		var serverToken []byte

		for _, hValue := range authHeaders {
			parts := strings.SplitN(hValue, " ", 2)
			pkg, err := security.ParseAuthPackage(parts[0])
			if err != nil {
				continue
			}

			// Prefer Negotiate over NTLM if multiple offered
			if selectedPackage == security.AuthNone || pkg == security.AuthNTLM {
				selectedPackage = pkg
				if len(parts) == 2 {
					serverToken, _ = base64.StdEncoding.DecodeString(parts[1])
				}
			}
		}

		if selectedPackage == security.AuthNone {
			// No supported auth package found
			logger.Warn("no supported auth package offered by upstream", "offered", authHeaders)
			_, err = responseHead.WriteTo(browser)
			return err
		}
		// serverTokenBytes is the size of the server's challenge token; the token
		// itself is a credential and is intentionally never logged.
		logger.Debug("selected auth package", "package", selectedPackage.String(), "serverTokenBytes", len(serverToken))

		if clientAuth == nil {
			clientAuth, err = security.NewClientAuth(selectedPackage)
			if err != nil {
				logger.Error("failed to initialize client auth", "package", selectedPackage.String(), "error", err)
				return fmt.Errorf("failed to initialize client auth: %w", err)
			}
			logger.Debug("initialized client auth context", "package", selectedPackage.String())
		}

		authDone, outputToken, err := clientAuth.Update(serverToken)
		if err != nil {
			logger.Error("auth update failed", "attempt", attempt, "error", err)
			return fmt.Errorf("auth update failed: %w", err)
		}
		logger.Debug("computed auth token", "attempt", attempt, "authDone", authDone, "outputTokenBytes", len(outputToken))

		// Prepare next request with the auth header expected by this upstream challenge.
		tokenBase64 := base64.StdEncoding.EncodeToString(outputToken)
		authValue = fmt.Sprintf("%s %s", selectedPackage.String(), tokenBase64)

		currentRequest.SetHeader(challenge.authorizationHeader, authValue)
		logger.Debug("set authorization header for next attempt", "header", challenge.authorizationHeader)

		if authDone {
			// Client side considers the handshake complete; replay the request
			// once more so the upstream can accept it and return a non-challenge
			// response.
			logger.Debug("handshake complete on client side, replaying request", "attempt", attempt)
			continue
		}

		// Handshake still in progress: loop and send the next token. Note some
		// proxies close the connection after a challenge if it is not
		// keep-alive; that surfaces as a read/write error on the next attempt.
		logger.Debug("handshake in progress, continuing", "attempt", attempt)
	}

}

func (m *ForwardProxyServerWithWindowsAuthentication) detectUpstreamAuthChallenge(responseHead models.HttpResponseHead) (upstreamAuthChallenge, bool) {
	if responseHead.StatusCode == 407 {
		return upstreamAuthChallenge{
			authenticateHeader:  "Proxy-Authenticate",
			authorizationHeader: "Proxy-Authorization",
		}, true
	}

	if responseHead.StatusCode == 401 && m.Treat401AsProxyAuthentication {
		return upstreamAuthChallenge{
			authenticateHeader:  "WWW-Authenticate",
			authorizationHeader: "Authorization",
		}, true
	}

	return upstreamAuthChallenge{}, false
}
