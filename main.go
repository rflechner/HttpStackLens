package main

import (
	"bufio"
	"fmt"
	"httpStackLens/http"
	"httpStackLens/proxy/middlewares"
	"httpStackLens/webui"
	"log"
	"net"
	"os"
	"time"
)

func main() {
	appContext, err := CreateOsSpecificProxyPipeline()
	if err != nil {
		log.Printf("Failed to configure proxy pipeline: %v\n", err)
		return
	}

	proxyServer := CreateProxyServer(appContext)

	stopChan := make(chan bool)

	hub := webui.ServeWebUi(9000, stopChan)
	ticker := time.NewTicker(1 * time.Second)

	go func() {
		for range ticker.C {
			hub.Publish("request_occurred", "coucou")
		}
	}()

	go proxyServer.Run()

	keyboard := bufio.NewReader(os.Stdin)

	go func() {
		fmt.Println("Type 'exit' to quit")
		for {
			line, _, _ := keyboard.ReadLine()
			if string(line) == "exit" {
				close(stopChan)
			}
		}
	}()

	select {
	case <-stopChan:
		proxyServer.Close()
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
