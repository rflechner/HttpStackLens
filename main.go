package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	configuration "httpStackLens/configuration"
	"httpStackLens/http/models"
	"httpStackLens/webui"
	"httpStackLens/webui/wasm/shared"
	"log"
	"os"
)

type ConsoleEventLogger struct{}

func (c *ConsoleEventLogger) LogEvent(event string) {
	fmt.Printf("Console Event: %s\n", event)
}

func (c *ConsoleEventLogger) LogRequest(id int, request models.ProxyRequest) {
	fmt.Printf("Console Request: %v\n", request.HttpRequestLine.String())
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
	event := shared.RequestEventDto{
		ID:      id,
		Method:  string(request.HttpRequestLine.HttpMethod),
		Host:    request.HttpRequestLine.Endpoint.Host,
		Port:    request.HttpRequestLine.Endpoint.Port,
		Path:    request.HttpRequestLine.Endpoint.PathAndQuery,
		Version: fmt.Sprintf("HTTP/%d.%d", request.HttpRequestLine.Version.Major, request.HttpRequestLine.Version.Minor),
	}
	jsonData, err := json.Marshal(event)
	if err != nil {
		log.Printf("Error marshaling request event: %v", err)
		return
	}
	c.Hub.Publish("request_occurred", string(jsonData))
}

func CreateWebUiEventLogger(hub *webui.Hub) *WebUiEventLogger {
	return &WebUiEventLogger{Hub: hub}
}

func main() {
	config := configuration.ReadConfiguration()

	appContext, err := CreateOsSpecificProxyPipeline(config)
	if err != nil {
		log.Printf("Failed to configure proxy pipeline: %v\n", err)
		return
	}

	stopChan := make(chan bool)

	webUiPort := 9000
	if config.WebUi.Port != 0 {
		webUiPort = config.WebUi.Port
	}

	hub := webui.ServeWebUi(webUiPort, stopChan)

	logger := CreateWebUiEventLogger(hub)
	proxyServer := CreateProxyServer(appContext, logger)

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
