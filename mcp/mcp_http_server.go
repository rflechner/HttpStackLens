package mcp

import (
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// newHandler builds the MCP server over deps. Every request it handles is
// traced in calls.
func newHandler(deps Dependencies, calls *CallLog) http.Handler {
	server := newServer(deps, calls)
	return mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server {
		return server
	}, nil)
}

// newServer registers every tool. It is the one list of what MCP clients are
// offered: the listener serves it and the Web UI catalog reads it back.
func newServer(deps Dependencies, calls *CallLog) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "http-stack-lens-mcp-server",
		Version: "1.0.0",
	}, nil)
	if calls != nil {
		server.AddReceivingMiddleware(calls.middleware)
	}

	// Both tools read local in-memory state only: without openWorldHint, MCP
	// clients would assume they reach the outside world.
	local := false
	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_requests",
		Description: "Search retained in-memory exchanges, newest first. Returns compact redacted summaries without headers or bodies. Repeat filters with next_cursor; missing responses are not proof of network failure.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: &local},
	}, deps.searchRequests)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_status",
		Description: "Read current local proxy, recording, persistence, HTTPS inspection and upstream state. Does not test connectivity or change settings. No traffic or credentials are returned.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: &local},
	}, deps.getStatus)

	registerComposerTools(server, deps)
	return server
}
