package middlewares

import (
	"encoding/base64"
	"fmt"
	"httpStackLens/http"
	"httpStackLens/http/models"
	"httpStackLens/security"
	"io"
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
	gatewayConnection, err := m.Forwarder.ConnectToGateway(browser, request)
	if err != nil {
		return err
	}
	defer gatewayConnection.Close()

	gateway := http.NewNetworkStream(gatewayConnection)

	var clientAuth *security.ClientAuth
	defer func() {
		if clientAuth != nil {
			clientAuth.Release()
		}
	}()

	var authValue string

	currentRequest := request
	for {
		// Send request to gateway
		_, err = currentRequest.WriteTo(gateway, true)
		if err != nil {
			return fmt.Errorf("failed to write request to gateway: %w", err)
		}

		// Read response head from gateway

		responseHead, err := http.ReadHttpResponse(gateway)
		if err != nil {
			return fmt.Errorf("failed to read response from gateway: %w", err)
		}

		challenge, ok := m.detectUpstreamAuthChallenge(responseHead)
		if !ok {
			_, err = responseHead.WriteTo(browser)
			if err != nil {
				return err
			}

			go io.Copy(browser, gateway)
			io.Copy(gateway, browser)

			fmt.Printf("Connection closed: %s\n", browser.RemoteAddr())
			// normal exit
			return nil
		}

		// Read response body from gateway
		_, err = http.ReadHttpResponseBody(gateway, responseHead)
		if err != nil {
			return fmt.Errorf("failed to read response body from gateway: %w", err)
		}

		authHeaders := responseHead.GetHeader(challenge.authenticateHeader)
		if len(authHeaders) == 0 {
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
			if selectedPackage == security.AuthNone || pkg == security.AuthNegotiate {
				selectedPackage = pkg
				if len(parts) == 2 {
					serverToken, _ = base64.StdEncoding.DecodeString(parts[1])
				}
			}
		}

		if selectedPackage == security.AuthNone {
			// No supported auth package found
			_, err = responseHead.WriteTo(browser)
			return err
		}

		if clientAuth == nil {
			clientAuth, err = security.NewClientAuth(selectedPackage)
			if err != nil {
				return fmt.Errorf("failed to initialize client auth: %w", err)
			}
		}

		authDone, outputToken, err := clientAuth.Update(serverToken)
		if err != nil {
			return fmt.Errorf("auth update failed: %w", err)
		}

		// Prepare next request with the auth header expected by this upstream challenge.
		tokenBase64 := base64.StdEncoding.EncodeToString(outputToken)
		authValue = fmt.Sprintf("%s %s", selectedPackage.String(), tokenBase64)

		currentRequest.SetHeader(challenge.authorizationHeader, authValue)

		if authDone {
			// We might need one more request to complete if the server didn't accept it yet,
			// but usually authDone on client means we sent the final token.
			// The loop will continue, send the request, and hopefully get a non-407.
			continue
		}

		// Note: We need to be careful about the gateway connection.
		// Some proxies might close it after 407 if not keep-alive.
		// But usually it's kept open for the handshake.
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
