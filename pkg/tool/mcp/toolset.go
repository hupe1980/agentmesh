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

	// Headers are HTTP headers passed to the MCP server for authentication
	// or session management. These headers are used when creating sessions.
	Headers map[string]string
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
// The view parameter provides access to graph state for context-aware discovery.
func (t *toolset) ListTools(ctx context.Context, _ graph.View) ([]tool.Tool, error) {
	session, err := t.sessionManager.CreateSession(ctx, t.opts.Headers)
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

// WithNamePrefix sets a prefix for all tool names discovered from the MCP server.
// This is useful to avoid name collisions when using multiple toolsets.
//
// Example:
//
//	toolset := mcp.NewToolset(factory, mcp.WithNamePrefix("server1"))
//	// Tools will be named: server1_toolname
func WithNamePrefix(prefix string) func(*ToolsetOptions) {
	return func(opts *ToolsetOptions) {
		opts.NamePrefix = prefix
	}
}

// WithHeaders sets HTTP headers to be sent when creating MCP sessions.
// Useful for authentication or passing session-specific metadata.
//
// Example:
//
//	toolset := mcp.NewToolset(factory, mcp.WithHeaders(map[string]string{
//	    "Authorization": "Bearer token123",
//	    "X-Session-ID": "abc-def",
//	}))
func WithHeaders(headers map[string]string) func(*ToolsetOptions) {
	return func(opts *ToolsetOptions) {
		opts.Headers = headers
	}
}
