package middlewares

import (
	"encoding/base64"
	"errors"
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
	newClientAuth                 func(security.AuthPackage) (upstreamClientAuth, error)
}

type upstreamClientAuth interface {
	Update([]byte) (bool, []byte, error)
	Release() error
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
	defer func() {
		if gatewayConnection != nil {
			_ = gatewayConnection.Close()
		}
	}()
	logger.Debug("connected to upstream gateway", "gateway", m.Forwarder.OutputProxy.Host)

	gateway := http.NewNetworkStream(gatewayConnection)

	newClientAuth := m.newClientAuth
	if newClientAuth == nil {
		newClientAuth = func(pkg security.AuthPackage) (upstreamClientAuth, error) {
			auth, err := security.NewClientAuth(pkg)
			if err != nil {
				return nil, err
			}
			return auth, nil
		}
	}
	var clientAuth upstreamClientAuth
	defer func() {
		if clientAuth != nil {
			clientAuth.Release()
		}
	}()

	currentRequest := upstreamAuthRequest(request)
	activePackage := security.AuthNone
	var activeChallenge upstreamAuthChallenge
	authComplete := false
	reconnections := 0

	setToken := func(token []byte) {
		value := fmt.Sprintf("%s %s", activePackage.String(), base64.StdEncoding.EncodeToString(token))
		setUpstreamAuthHeader(&currentRequest, activeChallenge.authorizationHeader, value)
	}
	restartAuth := func(reason string) error {
		if reconnections >= 2 {
			return fmt.Errorf("upstream proxy keeps closing the connection during authentication (2 reconnects exhausted)")
		}
		reconnections++
		logger.Debug("restarting upstream authentication on a new connection", "reason", reason, "reconnect", reconnections)
		_ = gatewayConnection.Close()
		if clientAuth != nil {
			_ = clientAuth.Release()
			clientAuth = nil
		}
		gatewayConnection, err = m.Forwarder.ConnectToGateway(browser, request)
		if err != nil {
			return err
		}
		gateway = http.NewNetworkStream(gatewayConnection)
		currentRequest = upstreamAuthRequest(request)
		clientAuth, err = newClientAuth(activePackage)
		if err != nil {
			return fmt.Errorf("restart client auth: %w", err)
		}
		// A server challenge belongs to its old connection. Begin with Type 1,
		// never replay a Type 3 token on a replacement connection.
		var token []byte
		authComplete, token, err = clientAuth.Update(nil)
		if err != nil {
			return fmt.Errorf("restart auth update: %w", err)
		}
		setToken(token)
		return nil
	}

	for attempt := 1; attempt <= 8; attempt++ {
		logger.Debug("sending request to gateway", "attempt", attempt)

		// Send request to gateway
		_, err = currentRequest.WriteTo(gateway, true)
		if err != nil {
			if activePackage != security.AuthNone && !authComplete && canRestartUpstreamAuthAfterEOF(request) {
				if restartErr := restartAuth("connection failed while sending an authentication token"); restartErr != nil {
					return restartErr
				}
				continue
			}
			logger.Error("failed to write request to gateway", "attempt", attempt, "error", err)
			return fmt.Errorf("failed to write request to gateway: %w", err)
		}

		// Read response head from gateway
		responseHead, err := http.ReadHttpResponse(gateway)
		if err != nil {
			if errors.Is(err, io.EOF) && activePackage != security.AuthNone && !authComplete && canRestartUpstreamAuthAfterEOF(request) {
				if err := restartAuth("EOF while awaiting an authentication response"); err != nil {
					return err
				}
				continue
			}
			logger.Error("failed to read response from gateway", "attempt", attempt, "error", err)
			return fmt.Errorf("failed to read response from gateway: %w", err)
		}
		logger.Debug("received response from gateway", "attempt", attempt, "status", responseHead.StatusCode,
			"connection", responseHead.GetHeader("Connection"), "proxyConnection", responseHead.GetHeader("Proxy-Connection"))

		challenge, ok := m.detectUpstreamAuthChallenge(responseHead)
		if !ok || authComplete {
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

			// Keep the same package for the lifetime of an authentication context.
			if activePackage != security.AuthNone && pkg != activePackage {
				continue
			}
			// Preserve the existing preference for NTLM when both are offered.
			if selectedPackage == security.AuthNone || pkg == security.AuthNTLM {
				selectedPackage = pkg
				serverToken = nil
				if len(parts) == 2 {
					serverToken, err = base64.StdEncoding.DecodeString(strings.TrimSpace(parts[1]))
					if err != nil {
						return fmt.Errorf("invalid upstream authentication token encoding")
					}
				}
			}
		}

		if selectedPackage == security.AuthNone {
			// No supported auth package found
			logger.Warn("no supported auth package offered by upstream")
			if _, err := responseHead.WriteTo(browser); err != nil {
				return err
			}
			_, err = io.Copy(browser, gateway)
			return err
		}
		activePackage = selectedPackage
		activeChallenge = challenge
		if upstreamAuthResponseClosesConnection(responseHead, request.HttpRequestLine.HttpMethod) {
			if err := restartAuth("authentication challenge closes the connection"); err != nil {
				return err
			}
			continue
		}
		if err := drainUpstreamAuthBody(gateway, responseHead, request.HttpRequestLine.HttpMethod); err != nil {
			return fmt.Errorf("failed to read authentication response body: %w", err)
		}
		// serverTokenBytes is the size of the server's challenge token; the token
		// itself is a credential and is intentionally never logged.
		logger.Debug("selected auth package", "package", selectedPackage.String(), "serverTokenBytes", len(serverToken))

		if clientAuth == nil {
			clientAuth, err = newClientAuth(selectedPackage)
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
		authComplete = authDone
		setToken(outputToken)
		logger.Debug("set authorization header for next attempt", "header", challenge.authorizationHeader)

		if authDone {
			// Client side considers the handshake complete; replay the request
			// once more so the upstream can accept it and return a non-challenge
			// response.
			logger.Debug("handshake complete on client side, replaying request", "attempt", attempt)
			continue
		}

		// Handshake still in progress: send the next token on this connection.
		logger.Debug("handshake in progress, continuing", "attempt", attempt)
	}
	return fmt.Errorf("upstream authentication exceeded 8 exchanges")
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
