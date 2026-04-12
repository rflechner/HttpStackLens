package logging

import (
	"encoding/json"
	"fmt"
	"httpStackLens/http/models"
	"httpStackLens/webui"
	"httpStackLens/webui/wasm/shared"
	"log"
)

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
