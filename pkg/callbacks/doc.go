// Package callbacks provides a flexible, composable callback system for intercepting
// and modifying model and tool invocations in AgentMesh.
//
// The callback system enables powerful extensions such as:
//   - Content guardrails and safety filters
//   - Response caching
//   - Metrics collection and tracing
//   - Input/output validation and transformation
//   - Policy enforcement
//
// # Callback Types
//
// Model Callbacks:
//   - BeforeModelCallback: Intercepts model requests before execution
//   - AfterModelCallback: Post-processes model responses
//   - OnModelErrorCallback: Handles model execution errors with fallback support
//
// Tool Callbacks:
//   - BeforeToolCallback: Intercepts tool invocations before execution
//   - AfterToolCallback: Post-processes tool responses
//   - OnToolErrorCallback: Handles tool execution errors
//
// # Usage Example
//
//	manager := callbacks.NewManager()
//	manager.RegisterBeforeModel(guardrails.BlockUnsafeContent)
//	manager.RegisterAfterModel(metrics.LogLatency)
//
//	// Use with ModelNode
//	node := agent.NewModelNode(
//	    agent.WithModel(model),
//	    agent.WithCallbacks(manager),
//	)
//
// # Thread Safety
//
// CallbackManager is thread-safe and supports concurrent registration and execution.
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
