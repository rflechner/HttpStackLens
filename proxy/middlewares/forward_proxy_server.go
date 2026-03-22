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
	clientAddr := browser.RemoteAddr().String()
	gateway, err := net.Dial("tcp", m.OutputProxy.Host)
	if err != nil {
		browser.Write([]byte(fmt.Sprintf("HTTP/1.1 502 Bad Gateway\r\n\r\nProxy %v not available", m.OutputProxy)))
		log.Println(err)
		return err
	}
	defer gateway.Close()

	_, err = gateway.Write([]byte(fmt.Sprintf(
		"CONNECT %s:%d HTTP/%d.%d\r\n",
		request.Connect.HostPort.Host, request.Connect.HostPort.Port, request.Connect.Version.Major, request.Connect.Version.Minor)))

	if err != nil {
		log.Printf("Could not send data to %v\n", m.OutputProxy)
		return err
	}

	for _, header := range request.Headers {
		if header.Name == "Proxy-Connection" {
			continue
		}
		_, err = gateway.Write([]byte(fmt.Sprintf("%s: %s\r\n", header.Name, header.Value)))
		if err != nil {
			log.Printf("Could not send header '%s' to %v\n", header.Name, m.OutputProxy)
			return err
		}
	}

	_, err = gateway.Write([]byte("\r\n"))
	if err != nil {
		log.Printf("Could not send end of request to %v\n", m.OutputProxy)
		return err
	}

	go io.Copy(browser, gateway)
	io.Copy(gateway, browser)

	fmt.Printf("Connection closed: %s\n", clientAddr)
	return nil
}
