package main

import (
	"flag"
	"fmt"
	"httpStackLens/http"
	"io"
	"log"
	"net"
	"net/url"
	"os"
)

func main() {
	port := flag.Int("port", 3128, "listening port")
	outputProxyUri := flag.String("output-proxy-uri", "", "URI to output proxy information")
	flag.Parse()

	var outputProxy *url.URL
	if len(*outputProxyUri) > 0 {
		u, err := url.Parse(*outputProxyUri)
		if err != nil {
			log.Printf("Invalid output proxy URI: %v\n", err)
		}
		outputProxy = u
	}

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

		if outputProxy != nil {
			fmt.Printf("Using output proxy: %s\n", outputProxy.String())
			go useOutputTunnel(conn, outputProxy)
		} else {
			go processTunnelRequest(conn)
		}
	}
}

func processTunnelRequest(browser net.Conn) {
	defer func(browser net.Conn) {
		_ = browser.Close()
	}(browser)

	clientAddr := browser.RemoteAddr().String()
	fmt.Printf("New connection from %s\n", clientAddr)

	request, err := http.ReadProxyRequest(browser)
	if err != nil {
		fmt.Printf("Error reading request from %s: %v\n", clientAddr, err)
		return
	}

	fmt.Printf("Request received: %v \n", request)

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

func useOutputTunnel(browser net.Conn, proxy *url.URL) {
	defer func(browser net.Conn) {
		_ = browser.Close()
	}(browser)

	clientAddr := browser.RemoteAddr().String()
	fmt.Printf("New connection from %s\n", clientAddr)

	request, err := http.ReadProxyRequest(browser)
	if err != nil {
		fmt.Printf("Error reading request from %s: %v\n", clientAddr, err)
		return
	}

	fmt.Printf("Request received: %v \n", request)

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
		log.Printf("Could not send data to %s\n", proxy)
		return
	}

	for _, header := range request.Headers {
		if header.Name == "Proxy-Connection" {
			continue
		}
		_, err = gateway.Write([]byte(fmt.Sprintf("%s: %s\r\n", header.Name, header.Value)))
		if err != nil {
			log.Printf("Could not send header '%s' to %s\n", header.Name, proxy)
			return
		}
	}

	_, err = gateway.Write([]byte("\r\n"))
	if err != nil {
		log.Printf("Could not send end of request to %s\n", proxy)
		return
	}

	go io.Copy(browser, gateway)
	io.Copy(gateway, browser)

	fmt.Printf("Connection closed: %s\n", clientAddr)
}
