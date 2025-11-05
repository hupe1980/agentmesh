package mcp

import (
	"context"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/tool"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolsetOptions holds configuration options for the MCP toolset.
type ToolsetOptions struct {
	// NamePrefix is prepended to each tool name, separated by an underscore.
	// This can be useful to avoid name collisions when multiple MCP toolsets
	// are used in the same agent.
	NamePrefix string
}

type toolset struct {
	sessionManager *SessionManager
	opts           ToolsetOptions
}

// NewToolset constructs a tool.Toolset backed by a Model Context Protocol server.
// It creates an internal SessionManager using the provided SessionFactory (e.g.,
// NewStdioSessionFactory). Tools are discovered at runtime via the MCP protocol.
func NewToolset(sessionFactory SessionFactory, optFns ...func(*ToolsetOptions)) tool.Toolset {
	opts := ToolsetOptions{}
	for _, fn := range optFns {
		fn(&opts)
	}

	return &toolset{
		sessionManager: NewSessionManager(sessionFactory),
		opts:           opts,
	}
}

// ListTools connects (or reuses a pooled connection) to the MCP server and
// streams available tools, converting each MCP tool descriptor into a
// tool.Tool proxy. The returned tools execute remotely via MCP when called.
func (t *toolset) ListTools(ctx context.Context, state graph.StateReader) ([]tool.Tool, error) {
	session, err := t.sessionManager.CreateSession(ctx, state, nil)
	if err != nil {
		return nil, err
	}

	toolsIter := session.Tools(ctx, &mcp.ListToolsParams{})

	tools := make([]tool.Tool, 0)
	for mcpTool, err := range toolsIter {
		if err != nil {
			return nil, err
		}

		t, err := NewTool(mcpTool, t.sessionManager, func(opts *ToolOptions) {
			opts.NamePrefix = t.opts.NamePrefix
		})
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
