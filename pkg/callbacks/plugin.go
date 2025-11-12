package callbacks

import (
	"context"
	"time"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
)

// Plugin defines the interface for extending AgentMesh with cross-cutting concerns.
// Plugins receive lifecycle hooks for graph, node, model, and tool operations.
//
// All hook methods should return nil error to continue execution. Returning a non-nil
// error stops execution and propagates the error to the caller.
//
// For model hooks (BeforeModel, AfterModel, OnModelError), returning a non-nil
// *model.Response short-circuits execution and uses that response.
//
// Embed NoopPlugin to inherit default no-op implementations for all hooks,
// then override only the hooks you need.
//
// Example:
//
//	type MyPlugin struct {
//	    NoopPlugin
//	    db *sql.DB
//	}
//
//	func NewMyPlugin(db *sql.DB) *MyPlugin {
//	    return &MyPlugin{db: db}
//	}
//
//	func (p *MyPlugin) BeforeModel(ctx context.Context, req *model.Request) (*model.Response, error) {
//	    // Check cache
//	    if cached := p.db.GetCached(req); cached != nil {
//	        return cached, nil // Short-circuit
//	    }
//	    return nil, nil // Continue to model
//	}
type Plugin interface {
	// Init initializes the plugin. Called once when the plugin is registered.
	// Use this for resource allocation, connection setup, etc.
	Init(ctx context.Context) error

	// Shutdown gracefully shuts down the plugin. Called when the graph is torn down.
	// Use this for cleanup, flushing buffers, closing connections, etc.
	Shutdown(ctx context.Context) error

	// OnGraphStart is called when graph execution begins.
	// graphID uniquely identifies this execution instance.
	OnGraphStart(ctx context.Context, graphID string) error

	// OnGraphComplete is called when graph execution finishes successfully.
	// stats contains execution metrics like duration, nodes visited, etc.
	OnGraphComplete(ctx context.Context, graphID string, stats GraphStats) error

	// OnGraphError is called when graph execution fails.
	OnGraphError(ctx context.Context, graphID string, err error) error

	// BeforeNode is called before a graph node executes.
	BeforeNode(ctx context.Context, nodeName string) error

	// AfterNode is called after a graph node executes successfully.
	// result contains the node's output and execution metadata.
	AfterNode(ctx context.Context, nodeName string, result NodeResult) error

	// BeforeModel is called before a model invocation.
	// Returning a non-nil *model.Response short-circuits the model call.
	//
	// Use cases: caching, content filtering, request validation, rate limiting.
	//
	// Example - Cache check:
	//
	//	func (p *CachePlugin) BeforeModel(ctx context.Context, req *model.Request) (*model.Response, error) {
	//	    if cached := p.cache.Get(req); cached != nil {
	//	        return cached, nil // Short-circuit
	//	    }
	//	    return nil, nil // Continue to model
	//	}
	BeforeModel(ctx context.Context, req *model.Request) (*model.Response, error)

	// AfterModel is called after a model invocation succeeds.
	// Returning a non-nil *model.Response replaces the original response.
	//
	// Use cases: response filtering, PII redaction, metrics, logging.
	//
	// Example - Content filter:
	//
	//	func (p *FilterPlugin) AfterModel(ctx context.Context, req *model.Request, resp *model.Response) (*model.Response, error) {
	//	    if containsToxicity(resp.Message.Parts().Text()) {
	//	        return &model.Response{Message: message.NewAI("[Content filtered]")}, nil
	//	    }
	//	    return nil, nil // Keep original
	//	}
	AfterModel(ctx context.Context, req *model.Request, resp *model.Response) (*model.Response, error)

	// OnModelError is called when a model invocation fails.
	// Returning a non-nil *model.Response with nil error replaces the error with that response.
	// Returning nil, nil propagates the original error.
	// Returning nil, newErr replaces the error.
	//
	// Use cases: fallback models, graceful degradation, retry coordination, error logging.
	//
	// Example - Fallback response:
	//
	//	func (p *FallbackPlugin) OnModelError(ctx context.Context, req *model.Request, err error) (*model.Response, error) {
	//	    return &model.Response{Message: message.NewAI("Service temporarily unavailable")}, nil
	//	}
	OnModelError(ctx context.Context, req *model.Request, err error) (*model.Response, error)

	// BeforeTool is called before a tool executes.
	// toolName identifies the tool, input is the tool's input parameters.
	BeforeTool(ctx context.Context, toolName string, input any) error

	// AfterTool is called after a tool executes successfully.
	// result contains the tool's output and execution metadata.
	AfterTool(ctx context.Context, toolName string, result ToolResult) error

	// OnToolError is called when a tool execution fails.
	OnToolError(ctx context.Context, toolName string, err error) error

	// OnStateChange is called when graph state is modified.
	// changes describes what state fields were added, updated, or removed.
	OnStateChange(ctx context.Context, changes StateChanges) error

	// OnMessage is called when a message is added to the conversation.
	OnMessage(ctx context.Context, msg message.Message) error
}

// NoopPlugin provides default no-op implementations for all Plugin hooks.
// Embed this in your plugin struct to only override the hooks you need.
//
// Example:
//
//	type MetricsPlugin struct {
//	    NoopPlugin
//	    registry prometheus.Registry
//	}
//
//	func (p *MetricsPlugin) BeforeModel(ctx context.Context, req *model.Request) (*model.Response, error) {
//	    p.registry.IncrementCounter("model_requests_total")
//	    return nil, nil
//	}
type NoopPlugin struct{}

// Init implements Plugin.Init as a no-op.
func (NoopPlugin) Init(ctx context.Context) error { return nil }

// Shutdown implements Plugin.Shutdown as a no-op.
func (NoopPlugin) Shutdown(ctx context.Context) error { return nil }

// OnGraphStart implements Plugin.OnGraphStart as a no-op.
func (NoopPlugin) OnGraphStart(ctx context.Context, graphID string) error { return nil }

// OnGraphComplete implements Plugin.OnGraphComplete as a no-op.
func (NoopPlugin) OnGraphComplete(ctx context.Context, graphID string, stats GraphStats) error {
	return nil
}

// OnGraphError implements Plugin.OnGraphError as a no-op.
func (NoopPlugin) OnGraphError(ctx context.Context, graphID string, err error) error { return nil }

// BeforeNode implements Plugin.BeforeNode as a no-op.
func (NoopPlugin) BeforeNode(ctx context.Context, nodeName string) error { return nil }

// AfterNode implements Plugin.AfterNode as a no-op.
func (NoopPlugin) AfterNode(ctx context.Context, nodeName string, result NodeResult) error {
	return nil
}

// BeforeModel implements Plugin.BeforeModel as a no-op.
func (NoopPlugin) BeforeModel(ctx context.Context, req *model.Request) (*model.Response, error) {
	return nil, nil
}

// AfterModel implements Plugin.AfterModel as a no-op.
func (NoopPlugin) AfterModel(ctx context.Context, req *model.Request, resp *model.Response) (*model.Response, error) {
	return nil, nil
}

// OnModelError implements Plugin.OnModelError as a no-op.
func (NoopPlugin) OnModelError(ctx context.Context, req *model.Request, err error) (*model.Response, error) {
	return nil, nil
}

// BeforeTool implements Plugin.BeforeTool as a no-op.
func (NoopPlugin) BeforeTool(ctx context.Context, toolName string, input any) error { return nil }

// AfterTool implements Plugin.AfterTool as a no-op.
func (NoopPlugin) AfterTool(ctx context.Context, toolName string, result ToolResult) error {
	return nil
}

// OnToolError implements Plugin.OnToolError as a no-op.
func (NoopPlugin) OnToolError(ctx context.Context, toolName string, err error) error { return nil }

// OnStateChange implements Plugin.OnStateChange as a no-op.
func (NoopPlugin) OnStateChange(ctx context.Context, changes StateChanges) error { return nil }

// OnMessage implements Plugin.OnMessage as a no-op.
func (NoopPlugin) OnMessage(ctx context.Context, msg message.Message) error { return nil }

// GraphStats contains metrics collected during graph execution.
type GraphStats struct {
	// Duration is the total execution time of the graph.
	Duration time.Duration

	// NodesVisited is the number of nodes that were executed.
	NodesVisited int

	// MessagesGenerated is the number of messages added to state.
	MessagesGenerated int

	// ToolInvocations is the number of tools that were called.
	ToolInvocations int
}

// NodeResult contains the output and metadata from a node execution.
type NodeResult struct {
	// Output is the value returned by the node (if any).
	Output any

	// Duration is how long the node took to execute.
	Duration time.Duration

	// Error is set if the node failed (only present in error hooks).
	Error error
}

// ToolResult contains the output and metadata from a tool execution.
type ToolResult struct {
	// Output is the value returned by the tool.
	Output any

	// Duration is how long the tool took to execute.
	Duration time.Duration

	// Error is set if the tool failed (only present in error hooks).
	Error error
}

// StateChanges describes modifications to the graph state.
type StateChanges struct {
	// Added contains state keys that were newly created.
	Added []string

	// Updated contains state keys whose values changed.
	Updated []string

	// Removed contains state keys that were deleted.
	Removed []string
}
