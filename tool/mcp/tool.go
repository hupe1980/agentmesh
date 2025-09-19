package mcp

import (
	"context"

	"github.com/hupe1980/agentmesh/core"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type tool struct {
	mcpTool        *mcp.Tool
	parameters     map[string]any
	sessionManager *SessionManager
}

// NewTool returns a core.Tool that proxies function calls to a Model Context Protocol
// server using the provided MCP tool declaration and a SessionManager.
func NewTool(mcpTool *mcp.Tool, sessionManager *SessionManager) (core.Tool, error) {
	parameters, err := core.SchemaToMap(mcpTool.InputSchema)
	if err != nil {
		return nil, err
	}

	return &tool{
		mcpTool:        mcpTool,
		parameters:     parameters,
		sessionManager: sessionManager,
	}, nil
}

// Name returns the MCP tool's name.
func (t *tool) Name() string {
	return t.mcpTool.Name
}

// Description returns the MCP tool's description.
func (t *tool) Description() string {
	return t.mcpTool.Description
}

// Parameters returns a JSON schema for arguments.
func (t *tool) Parameters() map[string]any {
	return t.parameters
}

// ProcessModelRequest registers this tool on the outgoing model request so
// the model can discover and call it.
func (t *tool) ProcessModelRequest(
	ctx context.Context,
	toolCtx core.ToolContext,
	req *core.ModelRequest,
) error {
	req.AddTool(t)
	return nil
}

// Call obtains (or reuses) an MCP ClientSession from the SessionManager and
// invokes the remote tool with the supplied arguments. Results are returned as
// structured content from the MCP server.
//
// Errors from session creation or the MCP invocation are propagated unchanged.
func (t *tool) Call(ctx context.Context, tc core.ToolContext, args map[string]any) (any, error) {
	session, err := t.sessionManager.CreateSession(ctx, tc, nil) // TODO: pass headers
	if err != nil {
		return nil, err
	}

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      t.mcpTool.Name,
		Arguments: args,
	})

	if err != nil {
		return nil, err
	}

	return res.StructuredContent, nil
}
