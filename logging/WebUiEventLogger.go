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

func (c *WebUiEventLogger) LogRequest(id int, correlationID string, request models.ProxyRequest) {
	line := request.HttpRequestLine
	tls := line.IsConnect()
	scheme := "http"
	if tls {
		scheme = "https"
	}
	c.PublishRequestEvent(shared.RequestEventDto{
		ID:            id,
		CorrelationID: correlationID,
		Method:        string(line.HttpMethod),
		Host:          line.Endpoint.Host,
		Port:          line.Endpoint.Port,
		Path:          line.Endpoint.PathAndQuery,
		Version:       fmt.Sprintf("HTTP/%d.%d", line.Version.Major, line.Version.Minor),
		Scheme:        scheme,
		Tls:           tls,
		// A CONNECT tunnel is encrypted but not decrypted; decrypted requests are
		// surfaced by the HTTPS interceptor with Decrypted set.
		Decrypted: false,
	})
}

// PublishRequestEvent streams a request event to the UI (SSE "request_occurred").
func (c *WebUiEventLogger) PublishRequestEvent(event shared.RequestEventDto) {
	jsonData, err := json.Marshal(event)
	if err != nil {
		log.Printf("Error marshaling request event: %v", err)
		return
	}
	c.Hub.Publish("request_occurred", string(jsonData))
}

// PublishResponseEvent streams a response event to the UI (SSE
// "response_occurred"), carrying the CorrelationID of the request it answers.
func (c *WebUiEventLogger) PublishResponseEvent(event shared.ResponseEventDto) {
	jsonData, err := json.Marshal(event)
	if err != nil {
		log.Printf("Error marshaling response event: %v", err)
		return
	}
	c.Hub.Publish("response_occurred", string(jsonData))
}

func CreateWebUiEventLogger(hub *webui.Hub) *WebUiEventLogger {
	return &WebUiEventLogger{Hub: hub}
}
