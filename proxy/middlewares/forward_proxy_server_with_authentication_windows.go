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
	Forwarder ForwardProxyServer
}

func (m *ForwardProxyServerWithWindowsAuthentication) HandleProxyRequest(browser net.Conn, request models.ProxyRequest) error {
	gateway, err := m.Forwarder.ConnectToGateway(browser, request)
	if err != nil {
		return err
	}
	defer gateway.Close()

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

		if responseHead.StatusCode != 407 {
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

		// It's a 407 Proxy Authentication Required
		authHeaders := responseHead.GetHeader("Proxy-Authenticate")
		if len(authHeaders) == 0 {
			// Forward 407 as is if no Proxy-Authenticate header
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

		// Prepare next request with Proxy-Authorization
		tokenBase64 := base64.StdEncoding.EncodeToString(outputToken)
		authValue = fmt.Sprintf("%s %s", selectedPackage.String(), tokenBase64)

		// Replace or add Proxy-Authorization header
		found := false
		for i := range currentRequest.Headers {
			if strings.EqualFold(currentRequest.Headers[i].Name, "Proxy-Authorization") {
				currentRequest.Headers[i].Value = authValue
				found = true
				break
			}
		}
		if !found {
			currentRequest.AddHeader("Proxy-Authorization", authValue)
		}

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
