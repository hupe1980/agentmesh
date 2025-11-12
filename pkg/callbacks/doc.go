// Package callbacks provides a flexible, composable plugin system for intercepting
// and modifying model and tool invocations in AgentMesh.
//
// The plugin system enables powerful extensions such as:
//   - Content guardrails and safety filters
//   - Response caching
//   - Metrics collection and tracing
//   - Input/output validation and transformation
//   - Policy enforcement
//
// # Plugin Lifecycle Hooks
//
// Lifecycle:
//   - Init: Initialize plugin resources
//   - Shutdown: Clean up plugin resources
//
// Graph Hooks:
//   - OnGraphStart: Called when graph execution begins
//   - OnGraphComplete: Called when graph execution succeeds
//   - OnGraphError: Called when graph execution fails
//
// Node Hooks:
//   - BeforeNode: Called before any node executes
//   - AfterNode: Called after node execution (success or failure)
//
// Model Hooks:
//   - BeforeModel: Intercepts model requests before execution (can short-circuit)
//   - AfterModel: Post-processes model responses (can transform)
//   - OnModelError: Handles model execution errors (can provide fallback)
//
// Tool Hooks:
//   - BeforeTool: Called before tool execution
//   - AfterTool: Called after tool execution
//   - OnToolError: Handles tool execution errors
//
// State Hooks:
//   - OnStateChange: Called when graph state changes
//   - OnMessage: Called when messages are added
//
// # Usage Example
//
//	manager := callbacks.NewPluginManager()
//	manager.Register(ctx, plugins.NewLoggingPlugin(logger, "MyAgent"))
//	manager.Register(ctx, plugins.NewMetricsPlugin(provider))
//
//	// Use with ModelNode
//	node := agent.NewModelNode(
//	    agent.WithModel(model),
//	    agent.WithCallbacks(manager),
//	)
//
// # Thread Safety
//
// PluginManager is thread-safe and supports concurrent registration and execution.
// All execution methods use read locks to allow concurrent invocations.
//
// # Error Handling
//
// Callbacks can return errors to stop execution. The callback system includes
// built-in panic recovery to prevent individual callback failures from crashing
// the system (enabled by default, can be disabled for testing).
//
// # Short-Circuiting
//
// Before* callbacks can short-circuit execution by returning a non-nil response.
// This is useful for caching, validation, or policy enforcement.
package callbacks
