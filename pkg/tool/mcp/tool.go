package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hupe1980/agentmesh/pkg/tool"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolOptions holds configuration options for an individual MCP tool.
type ToolOptions struct {
	// NamePrefix is prepended to the tool name, separated by an underscore.
	NamePrefix string
}

// mcpToolImpl is an implementation of tool.Tool that proxies calls to an MCP tool.
type mcpToolImpl struct {
	mcpTool        *mcp.Tool
	definition     *tool.Definition
	sessionManager *SessionManager
	opts           ToolOptions
}

// NewTool returns a tool.Tool that proxies function calls to a Model Context Protocol
// server using the provided MCP tool declaration and a SessionManager.
func NewTool(mcpToolDef *mcp.Tool, sessionManager *SessionManager, optFns ...func(*ToolOptions)) (tool.Tool, error) {
	opts := ToolOptions{}
	for _, fn := range optFns {
		fn(&opts)
	}

	parameters, err := schemaToMap(mcpToolDef.InputSchema)
	if err != nil {
		return nil, err
	}

	name := mcpToolDef.Name
	if opts.NamePrefix != "" {
		name = fmt.Sprintf("%s_%s", opts.NamePrefix, mcpToolDef.Name)
	}

	definition := &tool.Definition{
		Type: "function",
		Function: tool.FunctionDefinition{
			Name:        name,
			Description: mcpToolDef.Description,
			Parameters:  parameters,
		},
	}

	return &mcpToolImpl{
		mcpTool:        mcpToolDef,
		definition:     definition,
		sessionManager: sessionManager,
		opts:           opts,
	}, nil
}

// Name returns the MCP tool's name.
func (t *mcpToolImpl) Name() string {
	return t.definition.Function.Name
}

// Description returns the MCP tool's description.
func (t *mcpToolImpl) Description() string {
	return t.definition.Function.Description
}

// Definition returns the tool definition.
func (t *mcpToolImpl) Definition() *tool.Definition {
	return t.definition
}

// RawTool returns the underlying MCP tool descriptor.
func (t *mcpToolImpl) RawTool() *mcp.Tool {
	return t.mcpTool
}

// Call obtains (or reuses) an MCP ClientSession from the SessionManager and
// invokes the remote tool with the supplied arguments. Results are returned as
// structured content from the MCP server.
//
// Errors from session creation or the MCP invocation are propagated unchanged.
func (t *mcpToolImpl) Call(ctx context.Context, args string) (any, error) {
	session, err := t.sessionManager.CreateSession(ctx, nil, nil) // TODO: pass state and headers
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
