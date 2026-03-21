package main

import (
	"fmt"
	"goproxy/http"
	"net"
	"os"
)

func main() {
	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		fmt.Println("Error starting server:", err)
		os.Exit(1)
	}
	defer listener.Close()

	fmt.Println("Socket server started on port 8080")

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Error accepting connection:", err)
			continue
		}

		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()

	clientAddr := conn.RemoteAddr().String()
	fmt.Printf("New connection from %s\n", clientAddr)

	request, err := http.ReadProxyRequest(conn)
	if err != nil {
		fmt.Printf("Error reading request from %s: %v\n", clientAddr, err)
		return
	}

	fmt.Printf("Request received: %v \n", request)

	/*
		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			message := scanner.Text()
			fmt.Printf("[%s] %s\n", clientAddr, message)

			context := p.NewParsingContext(message)
			parser := parser.ConnectParser()
			result, err := parser(context)
			if err != nil {
				fmt.Printf("Error parsing message: %v\n", err)
				break
			}

			fmt.Printf("[Command] -> Connect to %s:%d\n", result.Result.HostPort.Host, result.Result.HostPort.Port)
		}

		if err := scanner.Err(); err != nil {
			fmt.Printf("Read error for %s: %v\n", clientAddr, err)
		}
	*/

	fmt.Printf("Connection closed: %s\n", clientAddr)
}
