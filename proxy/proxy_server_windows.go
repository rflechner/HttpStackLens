package proxy

import (
	"encoding/base64"
	"fmt"
	"httpStackLens/http"
	"httpStackLens/http/ast"
	"httpStackLens/security"
	"log"
	"net"
	"net/url"
	"slices"
	"strings"
)

func ConfigureOsSpecificProxyPipeline(outputProxy *url.URL, requireWindowsAuthentication bool) (Middleware, error) {
	basePipeline := ConfigureProxyPipelineBase(outputProxy)

	if requireWindowsAuthentication {
		return &WindowsAuthenticationServerMiddleware{
			NextMiddleware: basePipeline,
		}, nil
	}
	return basePipeline, nil
}

type WindowsAuthenticationServerMiddleware struct {
	NextMiddleware Middleware
}

func (m *WindowsAuthenticationServerMiddleware) HandleProxyRequest(browser net.Conn, request ast.ProxyRequest) error {
	defer func(browser net.Conn) {
		_ = browser.Close()
	}(browser)

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

	for {
		r, err := http.ReadProxyRequest(browser)
		if err != nil {
			fmt.Printf("Error reading request from %s: %v\n", clientAddr, err)
			return fmt.Errorf("Error reading request from %s: %v\n", clientAddr, err)
		}
		request = r
		fmt.Printf("Request received: %v \n", request)

		proxyAuthIndex := slices.IndexFunc(request.Headers, func(header ast.Header) bool {
			return header.Name == "Proxy-Authorization"
		})
		if proxyAuthIndex == -1 {
			// 407 — aks authentication
			_, err := browser.Write([]byte(
				"HTTP/1.1 407 Proxy Authentication Required\r\n" +
					"Proxy-Authenticate: NTLM\r\n" +
					//"Proxy-Authenticate: Negotiate\r\n" +
					//"Proxy-Authenticate: Kerberos\r\n" +
					"Proxy-Connection: keep-alive\r\n" +
					"Connection: keep-alive\r\n" +
					"Content-Length: 0\r\n" +
					"\r\n",
			))
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
			browser.Write([]byte("HTTP/1.1 407 Proxy Authentication Required\r\n\r\nInvalid token format\r\n"))
			log.Println(err)
			return fmt.Errorf("invalid token format: %v", err)
		}

		if len(parts) != 2 {
			browser.Write([]byte("HTTP/1.1 407 Proxy Authentication Required\r\n\r\nInvalid token format\r\n"))
			log.Println("Invalid token format")
			return fmt.Errorf("invalid token format")
		}

		token, err := base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			browser.Write([]byte("HTTP/1.1 407 Proxy Authentication Required\r\n\r\nInvalid token format\r\n"))
			log.Println(err)
			return fmt.Errorf("invalid token format: %v", err)
		}

		// Validate token
		if auth == nil {
			auth, err = security.NewServerAuth(authPackage)
			if err != nil {
				browser.Write([]byte("HTTP/1.1 500 Internal Server Error\r\n\r\nProxy server cannot authenticate\r\n"))
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
			browser.Write([]byte("HTTP/1.1 407 Proxy Authentication Required\r\n\r\nInvalid token\r\n"))
			log.Println(err)
			return fmt.Errorf("invalid token: %v", err)
		}
		if authDone == false {
			//browser.Write([]byte("HTTP/1.1 407 Proxy Authentication Required\r\n\r\nAuthentication failed\r\n"))
			responseToken := base64.StdEncoding.EncodeToString(outputToken)
			challengeResponse := fmt.Sprintf("HTTP/1.1 407 Proxy Authentication Required\r\n"+
				"Proxy-Authenticate: %s %s\r\n"+
				"Proxy-Connection: keep-alive\r\n"+
				"Content-Length: 0\r\n"+
				"\r\n", authPackage, responseToken)
			browser.Write([]byte(challengeResponse))
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
