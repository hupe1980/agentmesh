package plugin

import (
	"context"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/state"
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

	// BeforeNode is called before a graph node executes.
	// view provides read-only access to the current state snapshot.
	// Returning non-nil *graph.Command short-circuits node execution with that command (must include routing).
	//
	// Use cases: caching, conditional skipping, pre-computed results, circuit breakers.
	//
	// Example - Cache check:
	//
	//	func (p *CachePlugin) BeforeNode(ctx context.Context, nodeName string, view state.ReadView) (*graph.Command, error) {
	//	    if cached := p.cache.Get(nodeName, view); cached != nil {
	//	        return graph.End(cached), nil // Short-circuit with cached result
	//	    }
	//	    return nil, nil // Continue to node execution
	//	}
	BeforeNode(ctx context.Context, nodeName string, view state.ReadView) (*graph.Command, error)

	// AfterNode is called after a graph node executes successfully.
	// view provides read-only access to the state after node execution.
	// updates contains the state modifications produced by the node (mutable map).
	//
	// Use cases: state enrichment, audit logging, metrics collection, result transformation.
	//
	// Example - Add metadata:
	//
	//	func (p *MetadataPlugin) AfterNode(ctx context.Context, nodeName string, view state.ReadView, updates state.Updates) error {
	//	    updates["_last_node"] = nodeName
	//	    updates["_timestamp"] = time.Now()
	//	    return nil
	//	}
	AfterNode(ctx context.Context, nodeName string, view state.ReadView, updates state.Updates) error

	// OnNodeError is called when a node execution fails (after all retries).
	//
	// Use cases: error logging, fallback handlers, alerting, error recovery, circuit breakers.
	//
	// Example - Log and track:
	//
	//	func (p *ErrorPlugin) OnNodeError(ctx context.Context, nodeName string, err error) error {
	//	    log.Printf("❌ Node %s failed: %v", nodeName, err)
	//	    p.failureTracker.Increment(nodeName)
	//	    return nil
	//	}
	OnNodeError(ctx context.Context, nodeName string, err error) error

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
	// result contains the tool's output.
	AfterTool(ctx context.Context, toolName string, result any) error

	// OnToolError is called when a tool execution fails.
	OnToolError(ctx context.Context, toolName string, err error) error

	// OnStateChange is called when a node modifies graph state.
	// nodeName identifies the node that produced the updates.
	// updates contains the state modifications (can be inspected or logged, but not modified).
	//
	// Use cases: audit logging, state tracking, debugging, metrics collection.
	//
	// Example - Audit log:
	//
	//	func (p *AuditPlugin) OnStateChange(ctx context.Context, nodeName string, updates state.Updates) error {
	//	    p.logger.Printf("Node %s modified %d keys: %v", nodeName, len(updates), maps.Keys(updates))
	//	    return nil
	//	}
	OnStateChange(ctx context.Context, nodeName string, updates state.Updates) error
}
