package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hupe1980/agentmesh/core"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolOptions holds configuration options for an individual MCP tool.
type ToolOptions struct {
	// NamePrefix is prepended to the tool name, separated by an underscore.
	NamePrefix string
}

// tool is an implementation of core.Tool that proxies calls to an MCP tool.
type tool struct {
	mcpTool        *mcp.Tool
	parameters     map[string]any
	sessionManager *SessionManager
	opts           ToolOptions
}

// NewTool returns a core.Tool that proxies function calls to a Model Context Protocol
// server using the provided MCP tool declaration and a SessionManager.
func NewTool(mcpTool *mcp.Tool, sessionManager *SessionManager, optFns ...func(*ToolOptions)) (core.Tool, error) {
	opts := ToolOptions{}
	for _, fn := range optFns {
		fn(&opts)
	}

	parameters, err := schemaToMap(mcpTool.InputSchema)
	if err != nil {
		return nil, err
	}

	return &tool{
		mcpTool:        mcpTool,
		parameters:     parameters,
		sessionManager: sessionManager,
		opts:           opts,
	}, nil
}

// Name returns the MCP tool's name.
func (t *tool) Name() string {
	return fmt.Sprintf("%s_%s", t.opts.NamePrefix, t.mcpTool.Name)
}

// Description returns the MCP tool's description.
func (t *tool) Description() string {
	return t.mcpTool.Description
}

// Parameters returns a JSON schema for arguments.
func (t *tool) Parameters() map[string]any {
	return t.parameters
}

// RawTool returns the underlying MCP tool descriptor.
func (t *tool) RawTool() *mcp.Tool {
	return t.mcpTool
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
func (t *tool) Call(ctx context.Context, tc core.ToolContext, args string) (any, error) {
	session, err := t.sessionManager.CreateSession(ctx, tc, nil) // TODO: pass headers
	if err != nil {
		return nil, err
	}

	var arguments any
	if strings.TrimSpace(args) != "" {
		var parsed any
		if err := json.Unmarshal([]byte(args), &parsed); err != nil {
			return nil, fmt.Errorf("mcp: decode tool arguments: %w", err)
		}
		arguments = parsed
	}

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      t.mcpTool.Name,
		Arguments: arguments,
	})

	if err != nil {
		return nil, err
	}

	return res.StructuredContent, nil
}

// schemaToMap normalizes the various schema representations supported by the MCP SDK
// into a generic map[string]any suitable for AgentMesh tool definitions.
func schemaToMap(schema any) (map[string]any, error) {
	if schema == nil {
		return nil, nil
	}

	switch v := schema.(type) {
	case map[string]any:
		return v, nil
	case *jsonschema.Schema:
		return marshalSchema(v)
	case jsonschema.Schema:
		return marshalSchema(&v)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("mcp: marshal schema (%T): %w", v, err)
		}

		var out map[string]any
		if err := json.Unmarshal(b, &out); err != nil {
			return nil, fmt.Errorf("mcp: decode schema bytes: %w", err)
		}

		return out, nil
	}
}

func marshalSchema(s *jsonschema.Schema) (map[string]any, error) {
	if s == nil {
		return nil, nil
	}

	b, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("mcp: marshal json schema: %w", err)
	}

	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("mcp: decode schema bytes: %w", err)
	}

	return out, nil
}
