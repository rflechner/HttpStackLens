package middlewares

import (
	"fmt"
	"httpStackLens/http/models"
	"io"
	"log"
	"net"
	"net/url"
)

type ForwardProxyServer struct {
	OutputProxy url.URL
}

func (m *ForwardProxyServer) HandleProxyRequest(browser net.Conn, request models.ProxyRequest) error {
	gateway, err := m.ConnectToGateway(browser, request)
	if err != nil {
		return err
	}

	defer gateway.Close()

	return m.ForwardToGateway(browser, gateway, request)
}

func (m *ForwardProxyServer) ConnectToGateway(browser net.Conn, request models.ProxyRequest) (net.Conn, error) {
	gateway, err := net.Dial("tcp", m.OutputProxy.Host)
	if err != nil {
		browser.Write([]byte(fmt.Sprintf("HTTP/1.1 502 Bad Gateway\r\n\r\nProxy %v not available", m.OutputProxy)))
		log.Println(err)
		return nil, err
	}

	return gateway, nil
}

func (m *ForwardProxyServer) ForwardToGateway(browser net.Conn, gateway net.Conn, request models.ProxyRequest) error {
	clientAddr := browser.RemoteAddr().String()

	_, err := request.WriteTo(gateway, true)
	if err != nil {
		log.Printf("Could not send request to %v\n", m.OutputProxy)
		return err
	}

	go io.Copy(browser, gateway)
	io.Copy(gateway, browser)

	fmt.Printf("Connection closed: %s\n", clientAddr)
	return nil
}
