package callbacks

import (
	"context"
	"sync"

	"github.com/hupe1980/agentmesh/pkg/message"
)

// Manager orchestrates callback registration and execution with thread-safety.
// It maintains separate lists for each callback type and executes them in registration order.
//
// All methods are safe for concurrent use. Callbacks are executed sequentially in the order
// they were registered. If any callback returns an error, execution stops and that error
// is returned immediately.
type Manager struct {
	mu sync.RWMutex

	beforeModel  []BeforeModelCallback
	afterModel   []AfterModelCallback
	onModelError []OnModelErrorCallback

	beforeTool  []BeforeToolCallback
	afterTool   []AfterToolCallback
	onToolError []OnToolErrorCallback
}

// NewManager creates a new callback manager with no registered callbacks.
func NewManager() *Manager {
	return &Manager{
		beforeModel:  []BeforeModelCallback{},
		afterModel:   []AfterModelCallback{},
		onModelError: []OnModelErrorCallback{},
		beforeTool:   []BeforeToolCallback{},
		afterTool:    []AfterToolCallback{},
		onToolError:  []OnToolErrorCallback{},
	}
}

// RegisterBeforeModel adds a callback to be invoked before model execution.
// Callbacks are executed in registration order.
func (m *Manager) RegisterBeforeModel(cb BeforeModelCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.beforeModel = append(m.beforeModel, cb)
}

// RegisterAfterModel adds a callback to be invoked after model execution.
// Callbacks are executed in registration order.
func (m *Manager) RegisterAfterModel(cb AfterModelCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.afterModel = append(m.afterModel, cb)
}

// RegisterOnModelError adds a callback to be invoked when a model execution fails.
// Callbacks are executed in registration order.
func (m *Manager) RegisterOnModelError(cb OnModelErrorCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onModelError = append(m.onModelError, cb)
}

// RegisterBeforeTool adds a callback to be invoked before tool execution.
// Callbacks are executed in registration order.
func (m *Manager) RegisterBeforeTool(cb BeforeToolCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.beforeTool = append(m.beforeTool, cb)
}

// RegisterAfterTool adds a callback to be invoked after tool execution.
// Callbacks are executed in registration order.
func (m *Manager) RegisterAfterTool(cb AfterToolCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.afterTool = append(m.afterTool, cb)
}

// RegisterOnToolError adds a callback to be invoked when a tool execution fails.
// Callbacks are executed in registration order.
func (m *Manager) RegisterOnToolError(cb OnToolErrorCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onToolError = append(m.onToolError, cb)
}

// ExecuteBeforeModel runs all registered BeforeModel callbacks in order.
// If any callback returns a non-nil message, execution stops and that message is returned.
// If any callback returns an error, execution stops and that error is returned.
//
// Returns:
//   - message.Message: non-nil if a callback short-circuited with a response
//   - error: non-nil if a callback failed
func (m *Manager) ExecuteBeforeModel(ctx context.Context, messages []message.Message) (message.Message, error) {
	m.mu.RLock()
	callbacks := m.beforeModel
	m.mu.RUnlock()

	for _, cb := range callbacks {
		result, err := safeExecuteBeforeModel(ctx, cb, messages)
		if err != nil {
			return nil, err
		}
		if result != nil {
			return result, nil // Short-circuit
		}
	}

	return nil, nil
}

// ExecuteAfterModel runs all registered AfterModel callbacks in order.
// Each callback may transform the response. The final (possibly transformed) response is returned.
// If any callback returns an error, execution stops and that error is returned.
//
// Returns:
//   - message.Message: the final response (original or transformed)
//   - error: non-nil if a callback failed
func (m *Manager) ExecuteAfterModel(ctx context.Context, messages []message.Message, response message.Message) (message.Message, error) {
	m.mu.RLock()
	callbacks := m.afterModel
	m.mu.RUnlock()

	current := response
	for _, cb := range callbacks {
		transformed, err := safeExecuteAfterModel(ctx, cb, messages, current)
		if err != nil {
			return nil, err
		}
		if transformed != nil {
			current = transformed
		}
	}

	return current, nil
}

// ExecuteOnModelError runs all registered OnModelError callbacks in order.
// Callbacks can provide fallback responses or transform errors.
// If a callback returns a non-nil message, that becomes the final response and execution stops.
// If a callback returns a non-nil error, that becomes the final error and execution stops.
//
// Returns:
//   - message.Message: non-nil if a callback provided a fallback response
//   - error: the final error (original or transformed)
func (m *Manager) ExecuteOnModelError(ctx context.Context, messages []message.Message, err error) (message.Message, error) {
	m.mu.RLock()
	callbacks := m.onModelError
	m.mu.RUnlock()

	currentErr := err
	for _, cb := range callbacks {
		result, newErr := safeExecuteOnModelError(ctx, cb, messages, currentErr)
		if result != nil {
			return result, nil // Fallback provided
		}
		if newErr != nil {
			currentErr = newErr
		}
	}

	return nil, currentErr
}

// ExecuteBeforeTool runs all registered BeforeTool callbacks in order.
// If any callback returns a non-nil result, execution stops and that result is returned.
// If any callback returns an error, execution stops and that error is returned.
//
// Returns:
//   - any: non-nil if a callback short-circuited with a result
//   - error: non-nil if a callback failed
func (m *Manager) ExecuteBeforeTool(ctx context.Context, call message.ToolCall) (any, error) {
	m.mu.RLock()
	callbacks := m.beforeTool
	m.mu.RUnlock()

	for _, cb := range callbacks {
		result, err := safeExecuteBeforeTool(ctx, cb, call)
		if err != nil {
			return nil, err
		}
		if result != nil {
			return result, nil // Short-circuit
		}
	}

	return nil, nil
}

// ExecuteAfterTool runs all registered AfterTool callbacks in order.
// Each callback may transform the result. The final (possibly transformed) result is returned.
// If any callback returns an error, execution stops and that error is returned.
//
// Returns:
//   - any: the final result (original or transformed)
//   - error: non-nil if a callback failed
func (m *Manager) ExecuteAfterTool(ctx context.Context, call message.ToolCall, result any) (any, error) {
	m.mu.RLock()
	callbacks := m.afterTool
	m.mu.RUnlock()

	current := result
	for _, cb := range callbacks {
		transformed, err := safeExecuteAfterTool(ctx, cb, call, current)
		if err != nil {
			return nil, err
		}
		if transformed != nil {
			current = transformed
		}
	}

	return current, nil
}

// ExecuteOnToolError runs all registered OnToolError callbacks in order.
// Callbacks can provide fallback results or transform errors.
// If a callback returns a non-nil result, that becomes the final result and execution stops.
// If a callback returns a non-nil error, that becomes the final error and execution stops.
//
// Returns:
//   - any: non-nil if a callback provided a fallback result
//   - error: the final error (original or transformed)
func (m *Manager) ExecuteOnToolError(ctx context.Context, call message.ToolCall, err error) (any, error) {
	m.mu.RLock()
	callbacks := m.onToolError
	m.mu.RUnlock()

	currentErr := err
	for _, cb := range callbacks {
		result, newErr := safeExecuteOnToolError(ctx, cb, call, currentErr)
		if result != nil {
			return result, nil // Fallback provided
		}
		if newErr != nil {
			currentErr = newErr
		}
	}

	return nil, currentErr
}

// HasBeforeModelCallbacks returns true if any BeforeModel callbacks are registered.
func (m *Manager) HasBeforeModelCallbacks() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.beforeModel) > 0
}

// HasAfterModelCallbacks returns true if any AfterModel callbacks are registered.
func (m *Manager) HasAfterModelCallbacks() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.afterModel) > 0
}

// HasOnModelErrorCallbacks returns true if any OnModelError callbacks are registered.
func (m *Manager) HasOnModelErrorCallbacks() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.onModelError) > 0
}

// HasBeforeToolCallbacks returns true if any BeforeTool callbacks are registered.
func (m *Manager) HasBeforeToolCallbacks() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.beforeTool) > 0
}

// HasAfterToolCallbacks returns true if any AfterTool callbacks are registered.
func (m *Manager) HasAfterToolCallbacks() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.afterTool) > 0
}

// HasOnToolErrorCallbacks returns true if any OnToolError callbacks are registered.
func (m *Manager) HasOnToolErrorCallbacks() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.onToolError) > 0
}
