package engine

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/core"
)

// CallbackType defines the specific lifecycle points where callbacks can be executed.
//
// Callbacks provide a flexible mechanism for hooking into the engine's execution
// pipeline without modifying core logic. Each type represents a specific point
// in the execution lifecycle where custom logic can be injected.
//
// Available callback types:
//   - BeforeAgent/AfterAgent: Around complete agent execution
//   - BeforeModel/AfterModel: Around LLM model interactions
//   - BeforeTool/AfterTool: Around individual tool executions
//   - OnError: When errors occur during execution
//   - OnStateChange: When session state is modified
//
// Callbacks are executed synchronously and can influence execution flow
// by returning errors that terminate the operation.
type CallbackType string

const (
	// CallbackBeforeAgent is triggered before an agent begins execution.
	// Use for setup, validation, or instrumentation.
	CallbackBeforeAgent CallbackType = "before_agent"

	// CallbackAfterAgent is triggered after an agent completes execution.
	// Use for cleanup, metrics collection, or post-processing.
	CallbackAfterAgent CallbackType = "after_agent"

	// CallbackBeforeModel is triggered before LLM model interactions.
	// Use for request modification, caching, or rate limiting.
	CallbackBeforeModel CallbackType = "before_model"

	// CallbackAfterModel is triggered after LLM model interactions.
	// Use for response processing, logging, or metrics collection.
	CallbackAfterModel CallbackType = "after_model"

	// CallbackBeforeTool is triggered before tool execution.
	// Use for parameter validation, security checks, or auditing.
	CallbackBeforeTool CallbackType = "before_tool"

	// CallbackAfterTool is triggered after tool execution.
	// Use for result processing, logging, or side effect handling.
	CallbackAfterTool CallbackType = "after_tool"

	// CallbackOnError is triggered when errors occur during execution.
	// Use for error handling, alerting, or recovery mechanisms.
	CallbackOnError CallbackType = "on_error"

	// CallbackOnStateChange is triggered when session state is modified.
	// Use for validation, auditing, or reactive processing.
	CallbackOnStateChange CallbackType = "on_state_change"
)

// CallbackContext provides rich context information for callback execution.
//
// This structure contains all the information a callback might need to make
// decisions or perform actions during execution. It provides access to:
//   - Execution context and invocation details
//   - Current event being processed
//   - Agent identification information
//   - Callback type for conditional logic
//   - Extensible metadata for custom use cases
//
// The context is populated by the engine and passed to each callback,
// allowing callbacks to inspect and potentially modify the execution state.
type CallbackContext struct {
	// InvocationContext provides access to the full execution context,
	// including session data, services, and agent information.
	InvocationContext *core.InvocationContext

	// Event is the current event being processed. May be nil for
	// callbacks that don't operate on specific events.
	Event *core.Event

	// AgentID identifies the agent associated with this callback.
	// Useful for agent-specific logic or filtering.
	AgentID string

	// CallbackType indicates which callback type triggered this execution.
	// Allows shared callback implementations to behave differently
	// based on the execution phase.
	CallbackType CallbackType

	// Metadata provides extensible storage for custom callback data.
	// Can be used to pass additional context or configuration to callbacks.
	Metadata map[string]interface{}
}

// Callback defines the interface for execution lifecycle hooks.
//
// Callbacks provide a clean way to extend engine functionality without
// modifying core code. They can be used for:
//   - Logging and monitoring
//   - Validation and security checks
//   - Metrics collection and analytics
//   - Custom business logic integration
//   - Error handling and recovery
//
// Implementations should be:
//   - Fast: Callbacks run synchronously and can block execution
//   - Safe: Handle errors gracefully and avoid panics
//   - Stateless: Don't rely on mutable state between invocations
//   - Idempotent: Safe to call multiple times with same inputs
//
// Error Handling:
// Callbacks that return errors will terminate the associated operation.
// Use this mechanism for validation or to enforce business rules.
type Callback interface {
	// Type returns the callback type this implementation handles.
	// Used by the callback manager to route callbacks to appropriate handlers.
	Type() CallbackType

	// Execute performs the callback logic with the provided context.
	// Returning an error will terminate the associated operation.
	Execute(ctx context.Context, callbackCtx *CallbackContext) error
}

// FunctionCallback wraps a function as a callback implementation.
//
// This is a convenience wrapper that allows simple functions to be used
// as callbacks without implementing the full Callback interface. It's
// particularly useful for simple, stateless callback logic.
//
// Example:
//
//	loggingCallback := NewFunctionCallback(
//	    CallbackBeforeAgent,
//	    func(ctx context.Context, callbackCtx *CallbackContext) error {
//	        log.Printf("Starting agent: %s", callbackCtx.AgentID)
//	        return nil
//	    },
//	)
type FunctionCallback struct {
	callbackType CallbackType
	fn           func(ctx context.Context, callbackCtx *CallbackContext) error
}

// NewFunctionCallback creates a new function-based callback.
//
// Parameters:
//   - callbackType: The callback type this function should handle
//   - fn: The function to execute when the callback is triggered
//
// The function will be called with the appropriate context when the
// specified callback type is triggered during execution.
func NewFunctionCallback(
	callbackType CallbackType,
	fn func(ctx context.Context, callbackCtx *CallbackContext) error,
) *FunctionCallback {
	return &FunctionCallback{
		callbackType: callbackType,
		fn:           fn,
	}
}

// Type returns the callback type this function handles.
func (c *FunctionCallback) Type() CallbackType {
	return c.callbackType
}

// Execute calls the wrapped function with the provided context.
func (c *FunctionCallback) Execute(ctx context.Context, callbackCtx *CallbackContext) error {
	return c.fn(ctx, callbackCtx)
}

// CallbackManager orchestrates callback execution throughout the engine lifecycle.
//
// The manager provides a centralized registry for callbacks and ensures they
// are executed at the appropriate points during engine operation. It supports:
//   - Multiple callbacks per callback type
//   - Sequential execution with error propagation
//   - Type-safe callback routing
//   - Graceful error handling
//
// Callbacks are executed in registration order, and any callback returning
// an error will terminate execution and prevent subsequent callbacks from running.
//
// Thread Safety:
// The CallbackManager is not inherently thread-safe. If callbacks will be
// registered from multiple goroutines, external synchronization is required.
// However, once registration is complete, callback execution is safe for
// concurrent use.
type CallbackManager struct {
	callbacks map[CallbackType][]Callback
}

// NewCallbackManager creates a new callback manager instance.
//
// Returns a manager ready for callback registration and execution.
// The manager starts empty and callbacks must be registered before use.
func NewCallbackManager() *CallbackManager {
	return &CallbackManager{
		callbacks: make(map[CallbackType][]Callback),
	}
}

// RegisterCallback adds a callback to the manager for the specified type.
//
// Multiple callbacks can be registered for the same type and will be
// executed in registration order. Callbacks are not validated at
// registration time, so ensure they are properly implemented.
//
// Parameters:
//   - callback: The callback implementation to register
//
// Example:
//
//	manager := NewCallbackManager()
//	manager.RegisterCallback(loggingCallback)
//	manager.RegisterCallback(metricsCallback)
func (cm *CallbackManager) RegisterCallback(callback Callback) {
	callbackType := callback.Type()
	cm.callbacks[callbackType] = append(cm.callbacks[callbackType], callback)
}

// ExecuteCallbacks executes all registered callbacks for the specified type.
//
// Callbacks are executed sequentially in registration order. If any callback
// returns an error, execution stops immediately and the error is returned.
// Subsequent callbacks will not be executed.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - callbackType: The type of callbacks to execute
//   - callbackCtx: Context information for the callbacks
//
// Returns:
//   - error: The first error returned by any callback, or nil if all succeed
//
// Example:
//
//	err := manager.ExecuteCallbacks(ctx, CallbackBeforeAgent, callbackCtx)
//	if err != nil {
//	    return fmt.Errorf("callback failed: %w", err)
//	}
func (cm *CallbackManager) ExecuteCallbacks(
	ctx context.Context,
	callbackType CallbackType,
	callbackCtx *CallbackContext,
) error {
	callbacks, exists := cm.callbacks[callbackType]
	if !exists {
		return nil // No callbacks registered for this type
	}

	for _, callback := range callbacks {
		if err := callback.Execute(ctx, callbackCtx); err != nil {
			return err
		}
	}

	return nil
}

// LoggingCallback provides structured logging for engine lifecycle events.
//
// This callback implementation captures execution events and forwards them
// to a logging function. It's useful for debugging, monitoring, and audit trails.
//
// The callback formats events in a consistent manner and can be configured
// with different log levels or output formats through the logger function.
//
// Example:
//
//	logger := func(message string) {
//	    log.Printf("[ENGINE] %s", message)
//	}
//	callback := NewLoggingCallback(CallbackBeforeAgent, logger)
type LoggingCallback struct {
	callbackType CallbackType
	logger       func(message string)
}

// NewLoggingCallback creates a new logging callback.
//
// Parameters:
//   - callbackType: The callback type to handle
//   - logger: Function to call with formatted log messages
//
// The logger function will be called with formatted messages containing
// relevant context information about the execution event.
func NewLoggingCallback(callbackType CallbackType, logger func(message string)) *LoggingCallback {
	return &LoggingCallback{
		callbackType: callbackType,
		logger:       logger,
	}
}

// Type returns the callback type this logger handles.
func (c *LoggingCallback) Type() CallbackType {
	return c.callbackType
}

// Execute logs the execution event with context information.
//
// The log message includes the callback type, agent ID, and event details
// when available. If no logger function is configured, the callback
// silently succeeds.
func (c *LoggingCallback) Execute(ctx context.Context, callbackCtx *CallbackContext) error {
	if c.logger != nil {
		message := fmt.Sprintf("[%s] Agent: %s, Event: %v",
			c.callbackType, callbackCtx.AgentID, callbackCtx.Event)
		c.logger(message)
	}
	return nil
}

// StateValidationCallback validates session state changes during execution.
//
// This callback provides a mechanism to enforce business rules and data
// integrity constraints on session state modifications. It can be used to:
//   - Validate state schema compliance
//   - Enforce business rules and invariants
//   - Prevent unauthorized state modifications
//   - Log state changes for auditing purposes
//
// The validation function receives the state delta (only the changes)
// and can return an error to reject the modification and terminate
// the associated operation.
//
// Example:
//
//	validator := func(delta map[string]interface{}) error {
//	    if userId, ok := delta["user_id"]; ok {
//	        if userId == nil {
//	            return errors.New("user_id cannot be nil")
//	        }
//	    }
//	    return nil
//	}
//	callback := NewStateValidationCallback(validator)
type StateValidationCallback struct {
	validator func(stateDelta map[string]interface{}) error
}

// NewStateValidationCallback creates a new state validation callback.
//
// Parameters:
//   - validator: Function to validate state changes
//
// The validator function will be called with the state delta from events
// that modify session state. It should return an error if the state
// change should be rejected.
func NewStateValidationCallback(validator func(stateDelta map[string]interface{}) error) *StateValidationCallback {
	return &StateValidationCallback{
		validator: validator,
	}
}

// Type returns the callback type (always CallbackOnStateChange).
func (c *StateValidationCallback) Type() CallbackType {
	return CallbackOnStateChange
}

// Execute validates state changes from the event.
//
// If the event contains a state delta and a validator is configured,
// the validator function is called to check the proposed changes.
// Validation errors are returned to terminate the operation.
func (c *StateValidationCallback) Execute(ctx context.Context, callbackCtx *CallbackContext) error {
	if c.validator != nil && callbackCtx.Event != nil {
		if callbackCtx.Event.Actions.StateDelta != nil {
			return c.validator(callbackCtx.Event.Actions.StateDelta)
		}
	}
	return nil
}
