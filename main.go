package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"httpStackLens/http"
	"httpStackLens/http/ast"
	"httpStackLens/security"
	"io"
	"log"
	"net"
	"net/url"
	"os"
	"slices"
	"strings"
)

func main() {
	port := flag.Int("port", 3128, "listening port")
	outputProxyUri := flag.String("output-proxy-uri", "", "URI to output proxy information")                                                    // -output-proxy-uri=http://localhost:3129/
	requireNegotiate := flag.Bool("require-negotiate", false, "specifies that browsers need negotiate authentication (Windows supported only)") //-require-negotiate=true
	flag.Parse()

	var outputProxy *url.URL
	if len(*outputProxyUri) > 0 {
		u, err := url.Parse(*outputProxyUri)
		if err != nil {
			log.Printf("Invalid output proxy URI: %v\n", err)
			return
		}
		outputProxy = u
	}

	//spn := "HTTP/" + strings.Split(outputProxyUri, ":")[0]
	//spn := "HTTP/localhost"
	//token, err2 := getNtlmHeaderToken(spn)
	//if err2 != nil {
	//	fmt.Printf("Could not get NTLM token: %s", err2.Error())
	//}
	//fmt.Printf("NTLM token: %s\n", token)

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		fmt.Println("Error starting server:", err)
		os.Exit(1)
	}
	defer func(listener net.Listener) {
		err = listener.Close()
		if err != nil {
			log.Printf("Warning when closing browser connection: %v\n", err.Error())
		}
	}(listener)

	log.Printf("Socket server started on port %v\n", *port)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("Error accepting connection:", err)
			continue
		}

		go handleNegotiate(conn, *requireNegotiate, func(conn net.Conn, request ast.ProxyRequest) {
			if outputProxy != nil {
				fmt.Printf("Using output proxy: %s\n", outputProxy.String())
				useOutputTunnel(conn, request, *outputProxy)
			} else {
				processTunnelRequest(conn, request)
			}
		})
	}
}

func handleNegotiate(browser net.Conn, requireNegotiate bool, handleConnect func(net.Conn, ast.ProxyRequest)) {
	defer func(browser net.Conn) {
		_ = browser.Close()
	}(browser)

	clientAddr := browser.RemoteAddr().String()
	fmt.Printf("New connection from %s\n", clientAddr)

	var request ast.ProxyRequest
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

	if requireNegotiate {
		for {
			r, err := http.ReadProxyRequest(browser)
			if err != nil {
				fmt.Printf("Error reading request from %s: %v\n", clientAddr, err)
				return
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
					return
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
				return
			}

			if len(parts) != 2 {
				browser.Write([]byte("HTTP/1.1 407 Proxy Authentication Required\r\n\r\nInvalid token format\r\n"))
				log.Println("Invalid token format")
				return
			}

			token, err := base64.StdEncoding.DecodeString(parts[1])
			if err != nil {
				browser.Write([]byte("HTTP/1.1 407 Proxy Authentication Required\r\n\r\nInvalid token format\r\n"))
				log.Println(err)
				return
			}

			// Validate token
			if auth == nil {
				auth, err = security.NewServerAuth(authPackage)
				if err != nil {
					browser.Write([]byte("HTTP/1.1 500 Internal Server Error\r\n\r\nProxy server cannot authenticate\r\n"))
					log.Println(err)
					return
				}
			} else {
				if authPackage != auth.AuthPackage {
					log.Println("Token validation failed: auth package mismatch")
					return
				}
			}

			authDone, outputToken, err := auth.ValidateToken(token)
			if err != nil {
				browser.Write([]byte("HTTP/1.1 407 Proxy Authentication Required\r\n\r\nInvalid token\r\n"))
				log.Println(err)
				return
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

	} else {
		request, err := http.ReadProxyRequest(browser)
		if err != nil {
			fmt.Printf("Error reading request from %s: %v\n", clientAddr, err)
			return
		}
		fmt.Printf("Request received: %v \n", request)
	}

	fmt.Printf("Handle Request: %v \n", request)
	handleConnect(browser, request)
}

func processTunnelRequest(browser net.Conn, request ast.ProxyRequest) {
	clientAddr := browser.RemoteAddr().String()
	webServer, err := net.Dial("tcp", fmt.Sprintf("%s:%d", request.Connect.HostPort.Host, request.Connect.HostPort.Port))
	if err != nil {
		browser.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		log.Println(err)
		return
	}
	defer webServer.Close()

	browser.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	log.Printf("Connection established with %s:%d for %s\n", request.Connect.HostPort.Host, request.Connect.HostPort.Port, clientAddr)

	go io.Copy(browser, webServer)
	io.Copy(webServer, browser)

	fmt.Printf("Connection closed: %s\n", clientAddr)
}

func useOutputTunnel(browser net.Conn, request ast.ProxyRequest, proxy url.URL) {
	clientAddr := browser.RemoteAddr().String()
	gateway, err := net.Dial("tcp", proxy.Host)
	if err != nil {
		browser.Write([]byte(fmt.Sprintf("HTTP/1.1 502 Bad Gateway\r\n\r\nProxy %v not available", proxy)))
		log.Println(err)
		return
	}
	defer gateway.Close()

	_, err = gateway.Write([]byte(fmt.Sprintf(
		"CONNECT %s:%d HTTP/%d.%d\r\n",
		request.Connect.HostPort.Host, request.Connect.HostPort.Port, request.Connect.Version.Major, request.Connect.Version.Minor)))

	if err != nil {
		log.Printf("Could not send data to %v\n", proxy)
		return
	}

	for _, header := range request.Headers {
		if header.Name == "Proxy-Connection" {
			continue
		}
		_, err = gateway.Write([]byte(fmt.Sprintf("%s: %s\r\n", header.Name, header.Value)))
		if err != nil {
			log.Printf("Could not send header '%s' to %v\n", header.Name, proxy)
			return
		}
	}

	_, err = gateway.Write([]byte("\r\n"))
	if err != nil {
		log.Printf("Could not send end of request to %v\n", proxy)
		return
	}

	go io.Copy(browser, gateway)
	io.Copy(gateway, browser)

	fmt.Printf("Connection closed: %s\n", clientAddr)
}
