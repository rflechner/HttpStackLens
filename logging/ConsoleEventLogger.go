package logging

import (
	"fmt"
	"httpStackLens/http/models"
)

type ConsoleEventLogger struct{}

func (c *ConsoleEventLogger) LogEvent(event string) {
	fmt.Printf("Console Event: %s\n", event)
}

func (c *ConsoleEventLogger) LogRequest(id int, correlationID string, request models.ProxyRequest) {
	fmt.Printf("Console Request [%s]: %v\n", correlationID, request.HttpRequestLine.String())
}

func CreateConsoleEventLogger() ConsoleEventLogger {
	return ConsoleEventLogger{}
}
