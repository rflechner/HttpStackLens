package middlewares

import (
	"fmt"
	"httpStackLens/http/models"
	"io"
	"log"
	"net"
)

type TunnelServer struct {
}

func (m *TunnelServer) HandleProxyRequest(browser net.Conn, request models.ProxyRequest) error {
	clientAddr := browser.RemoteAddr().String()
	webServer, err := net.Dial("tcp", fmt.Sprintf("%s:%d", request.HttpRequestLine.Endpoint.Host, request.HttpRequestLine.Endpoint.Port))
	if err != nil {
		browser.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		log.Println(err)
		return err
	}
	defer webServer.Close()

	if request.HttpRequestLine.IsConnect() {
		browser.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
		log.Printf("Connection established with %s:%d for %s\n", request.HttpRequestLine.Endpoint.Host, request.HttpRequestLine.Endpoint.Port, clientAddr)
	} else {
		log.Printf("Sending request to %s:%d\n", request.HttpRequestLine.Endpoint.Host, request.HttpRequestLine.Endpoint.Port)
		request.WriteTo(webServer, false)
	}

	go io.Copy(browser, webServer)
	io.Copy(webServer, browser)

	fmt.Printf("Connection closed: %s\n", clientAddr)
	return nil
}
