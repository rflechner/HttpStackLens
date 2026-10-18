package mcp

import (
	"context"
	"fmt"
	"net/http"
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

// newHandler builds the MCP server over deps. Every request it handles is
// traced in calls.
func newHandler(deps Dependencies, calls *CallLog) http.Handler {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "http-stack-lens-mcp-server",
		Version: "1.0.0",
	}, nil)
	if calls != nil {
		server.AddReceivingMiddleware(calls.middleware)
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_requests",
		Description: "Search retained in-memory exchanges, newest first. Returns compact redacted summaries without headers or bodies. Repeat filters with next_cursor; missing responses are not proof of network failure.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, deps.searchRequests)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_status",
		Description: "Read current local proxy, recording, persistence, HTTPS inspection and upstream state. Does not test connectivity or change settings. No traffic or credentials are returned.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, deps.getStatus)

	registerComposerTools(server, deps)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "cityTime",
		Description: "Get the current time in NYC, San Francisco, or Boston",
	}, getTime)

	return mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server {
		return server
	}, nil)
}
