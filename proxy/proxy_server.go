package proxy

import (
	"fmt"
	"httpStackLens/http/ast"
	"io"
	"log"
	"net"
	"net/url"
)

func ConfigureProxyPipelineBase(outputProxy *url.URL) Middleware {
	if outputProxy != nil {
		return &ForwardProxyServer{*outputProxy}
	}
	return &TunnelServer{}
}

func ConfigureOsSpecificProxyPipeline(outputProxy *url.URL, requireWindowsAuthentication bool) (Middleware, error) {
	if requireWindowsAuthentication {
		return nil, fmt.Errorf("windows authentication is not supported")
	}
	return ConfigureProxyPipelineBase(outputProxy), nil
}

type Middleware interface {
	HandleProxyRequest(browser net.Conn, request ast.ProxyRequest) error
}

type TunnelServer struct {
}

func (m *TunnelServer) HandleProxyRequest(browser net.Conn, request ast.ProxyRequest) error {
	clientAddr := browser.RemoteAddr().String()
	webServer, err := net.Dial("tcp", fmt.Sprintf("%s:%d", request.Connect.HostPort.Host, request.Connect.HostPort.Port))
	if err != nil {
		browser.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		log.Println(err)
		return err
	}
	defer webServer.Close()

	browser.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	log.Printf("Connection established with %s:%d for %s\n", request.Connect.HostPort.Host, request.Connect.HostPort.Port, clientAddr)

	go io.Copy(browser, webServer)
	io.Copy(webServer, browser)

	fmt.Printf("Connection closed: %s\n", clientAddr)
	return nil
}

type ForwardProxyServer struct {
	OutputProxy url.URL
}

func (m *ForwardProxyServer) HandleProxyRequest(browser net.Conn, request ast.ProxyRequest) error {
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
