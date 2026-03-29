package main

import (
	"fmt"
	"httpStackLens/http"
	"httpStackLens/proxy/middlewares"
	"log"
	"net"
	"os"
)

func main() {
	app_context, err := CreateOsSpecificProxyPipeline()
	if err != nil {
		log.Printf("Failed to configure proxy pipeline: %v\n", err)
		return
	}

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", app_context.port))
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

	log.Printf("Socket server started on port %v\n", app_context.port)

	for {
		browser, err := listener.Accept()
		if err != nil {
			log.Println("Error accepting connection:", err)
			continue
		}
		fmt.Printf("New connection from %s\n", browser.RemoteAddr().String())
		go handleRequest(browser)(app_context.pipeline)
	}
}

func handleRequest(browser net.Conn) func(pipeline middlewares.Middleware) {
	request, err := http.ReadProxyRequest(browser)
	if err != nil {
		fmt.Printf("Error reading request from %s: %v\n", browser.RemoteAddr().String(), err)
		return func(pipeline middlewares.Middleware) {}
	}

	return func(pipeline middlewares.Middleware) {
		defer func(browser net.Conn) {
			_ = browser.Close()
		}(browser)
		err := pipeline.HandleProxyRequest(browser, request)
		if err != nil {
			fmt.Printf("Error handling request from %s: %v\n", browser.RemoteAddr().String(), err)
		}
	}
}
