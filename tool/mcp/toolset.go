package mcp

import (
	"context"

	"github.com/hupe1980/agentmesh/core"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type toolset struct {
	sessionManager *SessionManager
}

// NewToolset constructs a core.Toolset backed by a Model Context Protocol server.
// It creates an internal SessionManager using the provided SessionFactory (e.g.,
// NewStdioSessionFactory). Tools are discovered at runtime via the MCP protocol.
func NewToolset(sessionFactory SessionFactory) core.Toolset {
	return &toolset{
		sessionManager: NewSessionManager(sessionFactory),
	}
}

// ListTools connects (or reuses a pooled connection) to the MCP server and
// streams available tools, converting each MCP tool descriptor into a
// core.Tool proxy. The returned tools execute remotely via MCP when called.
func (t *toolset) ListTools(ctx context.Context, roCtx core.ReadonlyContext) ([]core.Tool, error) {
	session, err := t.sessionManager.CreateSession(ctx, roCtx, nil)
	if err != nil {
		return nil, err
	}

	toolsIter := session.Tools(ctx, &mcp.ListToolsParams{})

	tools := make([]core.Tool, 0)
	for tool, err := range toolsIter {
		if err != nil {
			return nil, err
		}

		t, err := NewTool(tool, t.sessionManager)
		if err != nil {
			return nil, err
		}

		tools = append(tools, t)
	}

	return tools, nil
}

// Close shuts down the internal SessionManager and closes any active sessions.
func (t *toolset) Close() error {
	return t.sessionManager.Close(context.Background())
}
