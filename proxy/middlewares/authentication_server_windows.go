package middlewares

import (
	"encoding/base64"
	"fmt"
	"httpStackLens/http"
	"httpStackLens/http/models"
	"httpStackLens/security"
	"log"
	"net"
	"slices"
	"strings"
)

type WindowsAuthenticationServerMiddleware struct {
	NextMiddleware Middleware
}

func (m *WindowsAuthenticationServerMiddleware) HandleProxyRequest(browser net.Conn, request models.ProxyRequest) error {
	clientAddr := browser.RemoteAddr().String()
	fmt.Printf("New connection from %s\n", clientAddr)

	var auth *security.ServerAuth

	defer func(auth *security.ServerAuth) {
		if auth == nil {
			return
		}

		err := auth.Release()
		if err != nil {
			log.Println(err)
		}
	}(auth)

	browserStream := http.NewNetworkStream(browser)

	firstLoop := true
	var err error
	for {
		if firstLoop == false {
			request, err = http.ReadProxyRequest(browserStream)
		}
		firstLoop = false
		if err != nil {
			fmt.Printf("Error reading request from %s: %v\n", clientAddr, err)
			return fmt.Errorf("Error reading request from %s: %v\n", clientAddr, err)
		}
		fmt.Printf("Request received: %v \n", request)

		proxyAuthIndex := slices.IndexFunc(request.Headers, func(header models.Header) bool {
			return header.Name == "Proxy-Authorization"
		})
		if proxyAuthIndex == -1 {
			_, err := m.send407Response(browser, "Invalid token format")
			if err != nil {
				log.Printf("Failed to write 407 response to %s: %v\n", clientAddr, err)
				return fmt.Errorf("Failed to write 407 response to %s: %v\n", clientAddr, err)
			}
			fmt.Printf("Sent 407 response to %s\n", clientAddr)
			continue
		}

		authHeader := request.Headers[proxyAuthIndex]
		parts := strings.SplitN(authHeader.Value, " ", 2)
		authPackage, err := security.ParseAuthPackage(parts[0])
		if err != nil {
			_, _ = m.sendInvalidTokenResponse(browser)
			log.Println(err)
			return fmt.Errorf("invalid token format: %v", err)
		}

		if len(parts) != 2 {
			_, _ = m.sendInvalidTokenResponse(browser)
			log.Println("Invalid token format")
			return fmt.Errorf("invalid token format")
		}

		token, err := base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			_, _ = m.sendInvalidTokenResponse(browser)
			log.Println(err)
			return fmt.Errorf("invalid token format: %v", err)
		}

		// Validate token
		if auth == nil {
			auth, err = security.NewServerAuth(authPackage)
			if err != nil {
				_, _ = m.send407Response(browser, "Proxy server cannot authenticate")
				log.Println(err)
				return fmt.Errorf("proxy server cannot authenticate: %v", err)
			}
		} else {
			if authPackage != auth.AuthPackage {
				log.Println("Token validation failed: auth package mismatch")
				return fmt.Errorf("token validation failed: auth package mismatch")
			}
		}

		authDone, outputToken, err := auth.ValidateToken(token)
		if err != nil {
			_, _ = m.sendInvalidTokenResponse(browser)
			log.Println(err)
			return fmt.Errorf("invalid token: %v", err)
		}
		if authDone == false {
			responseToken := base64.StdEncoding.EncodeToString(outputToken)
			_, _ = m.sendChallengeResponse(browser, authPackage, responseToken)
			fmt.Println("Challenge response sent")
			continue
		}

		if authDone == true {
			log.Println("Authentication successful")
			break
		}
	}

	fmt.Printf("Handle Request: %v \n", request)

	return m.NextMiddleware.HandleProxyRequest(browser, request)
}

func (m *WindowsAuthenticationServerMiddleware) sendInvalidTokenResponse(browser net.Conn) (int64, error) {
	return m.send407Response(browser, "Invalid token format")
}

func (m *WindowsAuthenticationServerMiddleware) sendEmpty407Response(browser net.Conn) (int64, error) {
	rs := models.HttpResponse{
		Head: models.HttpResponseHead{
			HttpVersion:       models.Version{Major: 1, Minor: 1},
			StatusCode:        407,
			StatusDescription: "Proxy Authentication Required",
			Headers: []models.Header{
				{Name: "Proxy-Authenticate", Value: "NTLM"},
				{Name: "Proxy-Connection", Value: "keep-alive"},
				{Name: "Connection", Value: "keep-alive"},
			},
		},
		Body: models.EmptyBody{},
	}
	return rs.WriteTo(browser)
}

func (m *WindowsAuthenticationServerMiddleware) send407Response(browser net.Conn, message string) (int64, error) {
	rs := models.HttpResponse{
		Head: models.HttpResponseHead{
			HttpVersion:       models.Version{Major: 1, Minor: 1},
			StatusCode:        407,
			StatusDescription: "Proxy Authentication Required",
			Headers: []models.Header{
				{Name: "Proxy-Authenticate", Value: "NTLM"},
				{Name: "Connection", Value: "keep-alive"},
				{Name: "Proxy-Connection", Value: "keep-alive"},
			},
		},
		Body: models.BodyString{Content: message},
	}
	return rs.WriteTo(browser)
}

func (m *WindowsAuthenticationServerMiddleware) sendChallengeResponse(browser net.Conn, authPackage security.AuthPackage, responseToken string) (int64, error) {
	rs := models.HttpResponse{
		Head: models.HttpResponseHead{
			HttpVersion:       models.Version{Major: 1, Minor: 1},
			StatusCode:        407,
			StatusDescription: "Proxy Authentication Required",
			Headers: []models.Header{
				{Name: "Proxy-Authenticate", Value: fmt.Sprintf("%s %s", authPackage.String(), responseToken)},
				{Name: "Connection", Value: "keep-alive"},
				{Name: "Proxy-Connection", Value: "keep-alive"},
			},
		},
		Body: models.EmptyBody{},
	}
	return rs.WriteTo(browser)
}
