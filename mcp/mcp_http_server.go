package mcp

import (
	"context"
	"errors"
	"fmt"
	"httpStackLens/configuration"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type GetTimeParams struct {
	City string `json:"city" jsonschema:"City to get time for (nyc, sf, or boston)"`
}

func getTime(ctx context.Context, req *mcp.CallToolRequest, params *GetTimeParams) (*mcp.CallToolResult, any, error) {
	// Define time zones for each city
	locations := map[string]string{
		"nyc":    "America/New_York",
		"sf":     "America/Los_Angeles",
		"boston": "America/New_York",
	}

	city := params.City
	if city == "" {
		city = "nyc" // Default to NYC
	}

	// Get the timezone.
	tzName, ok := locations[city]
	if !ok {
		return nil, nil, fmt.Errorf("unknown city: %s", city)
	}

	// Load the location.
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load timezone: %w", err)
	}

	// Get current time in that location.
	now := time.Now().In(loc)

	// Format the response.
	cityNames := map[string]string{
		"nyc":    "New York City",
		"sf":     "San Francisco",
		"boston": "Boston",
	}

	response := fmt.Sprintf("The current time in %s is %s",
		cityNames[city],
		now.Format(time.RFC3339))

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: response},
		},
	}, nil, nil
}

// Start starts the optional loopback listener. The caller owns the returned
// HTTP server and must close it when the application stops.
// A disabled service returns (nil, nil); startup failures never exit the process.
func Start(config configuration.MCPConfig) (*http.Server, error) {
	if !config.Enabled {
		return nil, nil
	}
	if config.Port < 1 || config.Port > 65535 {
		return nil, fmt.Errorf("MCP port must be between 1 and 65535, got %d", config.Port)
	}

	addr := net.JoinHostPort("localhost", strconv.Itoa(config.Port))
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("start MCP listener: %w", err)
	}
	server := &http.Server{Addr: addr, Handler: newHandler()}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("MCP server stopped", "error", err)
		}
	}()
	slog.Info("MCP server listening", "url", "http://"+addr)
	return server, nil
}

func newHandler() http.Handler {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "http-stack-lens-mcp-server",
		Version: "1.0.0",
	}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "cityTime",
		Description: "Get the current time in NYC, San Francisco, or Boston",
	}, getTime)

	return mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server {
		return server
	}, nil)
}
