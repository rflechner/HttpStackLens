package main

import (
	"fmt"
	"goproxy/http"
	"io"
	"log"
	"net"
	"os"
)

func main() {
	listener, err := net.Listen("tcp", ":3128")
	if err != nil {
		fmt.Println("Error starting server:", err)
		os.Exit(1)
	}
	defer listener.Close()

	fmt.Println("Socket server started on port 3128")

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Error accepting connection:", err)
			continue
		}

		go handleConnection(conn)
	}
}

func handleConnection(browser net.Conn) {
	defer browser.Close()

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
		log.Fatal(err)
	}
	defer webServer.Close()

	browser.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))

	go io.Copy(browser, webServer)
	io.Copy(webServer, browser)

	fmt.Printf("Connection closed: %s\n", clientAddr)
}
