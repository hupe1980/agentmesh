package callbacks

import (
	"context"
	"sync"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
)

// PluginManager orchestrates plugin registration and lifecycle with thread-safety.
type PluginManager struct {
	mu      sync.RWMutex
	plugins []Plugin
}

// NewPluginManager creates a new plugin manager with no registered plugins.
func NewPluginManager() *PluginManager {
	return &PluginManager{
		plugins: []Plugin{},
	}
}

// Register adds a plugin to the manager and initializes it.
func (pm *PluginManager) Register(ctx context.Context, plugin Plugin) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if err := plugin.Init(ctx); err != nil {
		return err
	}

	pm.plugins = append(pm.plugins, plugin)
	return nil
}

// Shutdown gracefully shuts down all registered plugins in reverse order.
func (pm *PluginManager) Shutdown(ctx context.Context) error {
	pm.mu.RLock()
	plugins := make([]Plugin, len(pm.plugins))
	copy(plugins, pm.plugins)
	pm.mu.RUnlock()

	for i := len(plugins) - 1; i >= 0; i-- {
		if err := plugins[i].Shutdown(ctx); err != nil {
			return err
		}
	}

	return nil
}

// ExecuteOnGraphStart runs all plugins OnGraphStart hooks.
func (pm *PluginManager) ExecuteOnGraphStart(ctx context.Context, graphID string) error {
	pm.mu.RLock()
	plugins := pm.plugins
	pm.mu.RUnlock()

	for _, p := range plugins {
		if err := safeExecuteOnGraphStart(ctx, p, graphID); err != nil {
			return err
		}
	}

	return nil
}

// ExecuteOnGraphComplete runs all plugins OnGraphComplete hooks.
func (pm *PluginManager) ExecuteOnGraphComplete(ctx context.Context, graphID string, stats GraphStats) error {
	pm.mu.RLock()
	plugins := pm.plugins
	pm.mu.RUnlock()

	for _, p := range plugins {
		if err := safeExecuteOnGraphComplete(ctx, p, graphID, stats); err != nil {
			return err
		}
	}

	return nil
}

// ExecuteOnGraphError runs all plugins OnGraphError hooks.
func (pm *PluginManager) ExecuteOnGraphError(ctx context.Context, graphID string, err error) error {
	pm.mu.RLock()
	plugins := pm.plugins
	pm.mu.RUnlock()

	for _, p := range plugins {
		if hookErr := safeExecuteOnGraphError(ctx, p, graphID, err); hookErr != nil {
			return hookErr
		}
	}

	return nil
}

// ExecuteBeforeNode runs all plugins BeforeNode hooks.
func (pm *PluginManager) ExecuteBeforeNode(ctx context.Context, nodeName string) error {
	pm.mu.RLock()
	plugins := pm.plugins
	pm.mu.RUnlock()

	for _, p := range plugins {
		if err := safeExecuteBeforeNode(ctx, p, nodeName); err != nil {
			return err
		}
	}

	return nil
}

// ExecuteAfterNode runs all plugins AfterNode hooks.
func (pm *PluginManager) ExecuteAfterNode(ctx context.Context, nodeName string, result NodeResult) error {
	pm.mu.RLock()
	plugins := pm.plugins
	pm.mu.RUnlock()

	for _, p := range plugins {
		if err := safeExecuteAfterNode(ctx, p, nodeName, result); err != nil {
			return err
		}
	}

	return nil
}

// ExecuteBeforeModel runs all plugins BeforeModel hooks.
func (pm *PluginManager) ExecuteBeforeModel(ctx context.Context, req *model.Request) (*model.Response, error) {
	pm.mu.RLock()
	plugins := pm.plugins
	pm.mu.RUnlock()

	for _, p := range plugins {
		resp, err := safeExecuteBeforeModel(ctx, p, req)
		if err != nil {
			return nil, err
		}
		if resp != nil {
			return resp, nil
		}
	}

	return nil, nil
}

// ExecuteAfterModel runs all plugins AfterModel hooks.
func (pm *PluginManager) ExecuteAfterModel(ctx context.Context, req *model.Request, resp *model.Response) (*model.Response, error) {
	pm.mu.RLock()
	plugins := pm.plugins
	pm.mu.RUnlock()

	current := resp
	for _, p := range plugins {
		transformed, err := safeExecuteAfterModel(ctx, p, req, current)
		if err != nil {
			return nil, err
		}
		if transformed != nil {
			current = transformed
		}
	}

	return current, nil
}

// ExecuteOnModelError runs all plugins OnModelError hooks.
func (pm *PluginManager) ExecuteOnModelError(ctx context.Context, req *model.Request, err error) (*model.Response, error) {
	pm.mu.RLock()
	plugins := pm.plugins
	pm.mu.RUnlock()

	currentErr := err
	for _, p := range plugins {
		resp, hookErr := safeExecuteOnModelError(ctx, p, req, currentErr)
		if resp != nil {
			return resp, nil
		}
		if hookErr != nil {
			currentErr = hookErr
		}
	}

	return nil, currentErr
}

// ExecuteBeforeTool runs all plugins BeforeTool hooks.
func (pm *PluginManager) ExecuteBeforeTool(ctx context.Context, toolName string, input any) error {
	pm.mu.RLock()
	plugins := pm.plugins
	pm.mu.RUnlock()

	for _, p := range plugins {
		if err := safeExecuteBeforeTool(ctx, p, toolName, input); err != nil {
			return err
		}
	}

	return nil
}

// ExecuteAfterTool runs all plugins AfterTool hooks.
func (pm *PluginManager) ExecuteAfterTool(ctx context.Context, toolName string, result ToolResult) error {
	pm.mu.RLock()
	plugins := pm.plugins
	pm.mu.RUnlock()

	for _, p := range plugins {
		if err := safeExecuteAfterTool(ctx, p, toolName, result); err != nil {
			return err
		}
	}

	return nil
}

// ExecuteOnToolError runs all plugins OnToolError hooks.
func (pm *PluginManager) ExecuteOnToolError(ctx context.Context, toolName string, err error) error {
	pm.mu.RLock()
	plugins := pm.plugins
	pm.mu.RUnlock()

	for _, p := range plugins {
		if hookErr := safeExecuteOnToolError(ctx, p, toolName, err); hookErr != nil {
			return hookErr
		}
	}

	return nil
}

// ExecuteOnStateChange runs all plugins OnStateChange hooks.
func (pm *PluginManager) ExecuteOnStateChange(ctx context.Context, changes StateChanges) error {
	pm.mu.RLock()
	plugins := pm.plugins
	pm.mu.RUnlock()

	for _, p := range plugins {
		if err := safeExecuteOnStateChange(ctx, p, changes); err != nil {
			return err
		}
	}

	return nil
}

// ExecuteOnMessage runs all plugins OnMessage hooks.
func (pm *PluginManager) ExecuteOnMessage(ctx context.Context, msg message.Message) error {
	pm.mu.RLock()
	plugins := pm.plugins
	pm.mu.RUnlock()

	for _, p := range plugins {
		if err := safeExecuteOnMessage(ctx, p, msg); err != nil {
			return err
		}
	}

	return nil
}

// HasPlugins returns true if any plugins are registered.
func (pm *PluginManager) HasPlugins() bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return len(pm.plugins) > 0
}
