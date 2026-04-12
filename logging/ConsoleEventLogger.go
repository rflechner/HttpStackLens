package logging

import (
	"fmt"
	"httpStackLens/http/models"
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
