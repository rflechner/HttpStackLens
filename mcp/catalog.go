package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"httpStackLens/webui/wasm/shared"
	"sort"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Tools lists the tools the server offers, whether the listener runs or not.
// They are read back through an in-memory MCP session rather than from a list
// kept on the side, so that the Web UI shows exactly what a client is sent:
// descriptions, annotations and the schemas the SDK derives from the Go types.
// The session is not traced.
func (c *Controller) Tools(ctx context.Context) ([]shared.McpToolDto, error) {
	c.mu.Lock()
	deps := c.deps
	c.mu.Unlock()

	serverTransport, clientTransport := sdk.NewInMemoryTransports()
	serverSession, err := newServer(deps, nil).Connect(ctx, serverTransport, nil)
	if err != nil {
		return nil, fmt.Errorf("list MCP tools: %w", err)
	}
	defer serverSession.Close()
	client := sdk.NewClient(&sdk.Implementation{Name: "httpstacklens-web-ui", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		return nil, fmt.Errorf("list MCP tools: %w", err)
	}
	defer session.Close()

	var tools []shared.McpToolDto
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			return nil, fmt.Errorf("list MCP tools: %w", err)
		}
		tools = append(tools, toolDto(tool))
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	return tools, nil
}

// toolDto applies the MCP defaults to the annotations: destructiveHint and
// openWorldHint are true when absent, and destructiveHint only means anything
// for a tool that is not read-only.
func toolDto(tool *sdk.Tool) shared.McpToolDto {
	dto := shared.McpToolDto{
		Name:        tool.Name,
		Title:       tool.Title,
		Description: tool.Description,
		Destructive: true,
		OpenWorld:   true,
	}
	if a := tool.Annotations; a != nil {
		if dto.Title == "" {
			dto.Title = a.Title
		}
		dto.ReadOnly = a.ReadOnlyHint
		dto.Idempotent = a.IdempotentHint
		if a.DestructiveHint != nil {
			dto.Destructive = *a.DestructiveHint
		}
		if a.OpenWorldHint != nil {
			dto.OpenWorld = *a.OpenWorldHint
		}
	}
	if dto.ReadOnly {
		dto.Destructive = false
	}
	dto.InputSchema = rawSchema(tool.InputSchema)
	dto.OutputSchema = rawSchema(tool.OutputSchema)
	return dto
}

func rawSchema(schema any) json.RawMessage {
	if schema == nil {
		return nil
	}
	data, err := json.Marshal(schema)
	if err != nil || string(data) == "null" {
		return nil
	}
	return data
}
