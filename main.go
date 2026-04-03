package main

import (
	"bufio"
	"fmt"
	"httpStackLens/http/models"
	"httpStackLens/webui"
	"log"
	"os"
)

type ConsoleEventLogger struct{}

func (c *ConsoleEventLogger) LogEvent(event string) {
	fmt.Printf("Console Event: %s\n", event)
}

func (c *ConsoleEventLogger) LogRequest(id int, request models.ProxyRequest) {
	fmt.Printf("Console Request: %v\n", request)
}

func CreateConsoleEventLogger() ConsoleEventLogger {
	return ConsoleEventLogger{}
}

type WebUiEventLogger struct {
	Hub *webui.Hub
}

func (c *WebUiEventLogger) LogEvent(event string) {
	c.Hub.Publish("event_occurred", event)
}

func (c *WebUiEventLogger) LogRequest(id int, request models.ProxyRequest) {
	c.Hub.Publish("request_occurred", request.HttpRequestLine.String())
}

func CreateWebUiEventLogger(hub *webui.Hub) *WebUiEventLogger {
	return &WebUiEventLogger{Hub: hub}
}

func main() {
	appContext, err := CreateOsSpecificProxyPipeline()
	if err != nil {
		log.Printf("Failed to configure proxy pipeline: %v\n", err)
		return
	}

	stopChan := make(chan bool)

	hub := webui.ServeWebUi(9000, stopChan)

	logger := CreateWebUiEventLogger(hub)
	proxyServer := CreateProxyServer(appContext, logger)

	//ticker := time.NewTicker(1 * time.Second)
	//
	//go func() {
	//	for range ticker.C {
	//		hub.Publish("request_occurred", "coucou")
	//	}
	//}()

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
