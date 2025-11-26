package callbacks

import (
	"context"
	"sync"

	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/plugin"
	"github.com/hupe1980/agentmesh/pkg/state"
)

// PluginManager orchestrates plugin registration and lifecycle with thread-safety.
//
// The PluginManager automatically implements interface contracts from other packages:
//   - graph.NodeCallbacks (BeforeNode, AfterNode, OnNodeError)
//   - graph.StateCallbacks (OnStateChange)
//   - model.ModelCallbacks (BeforeModel, AfterModel, OnModelError)
//   - tool.ToolCallbacks (BeforeTool, AfterTool, OnToolError)
//
// This design follows the Dependency Inversion Principle - each package defines
// the callback interface it needs, and PluginManager satisfies all of them without
// creating import cycles.
//
// Usage:
//
//	pm := callbacks.NewPluginManager()
//	pm.Register(ctx, myPlugin)
//
//	// Use with agents - automatic context injection
//	agent, _ := agent.NewReActAgent(model,
//	    agent.WithTools(tools...),
//	    agent.WithPluginManager(pm))
//
//	The plugin manager is injected into context by the wrapper and retrieved by:
//	- Nodes via callbacks.FromContext(ctx)
//	- Graph executors via graph.WithNodeCallbacks/WithStateCallbacks
type PluginManager struct {
	mu      sync.RWMutex
	plugins []plugin.Plugin
}

// NewPluginManager creates a new plugin manager with no registered plugins.
func NewPluginManager() *PluginManager {
	return &PluginManager{
		plugins: []plugin.Plugin{},
	}
}

// Register adds a plugin to the manager and initializes it.
func (pm *PluginManager) Register(ctx context.Context, p plugin.Plugin) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if err := p.Init(ctx); err != nil {
		return err
	}

	pm.plugins = append(pm.plugins, p)
	return nil
}

// Shutdown gracefully shuts down all registered plugins in reverse order.
func (pm *PluginManager) Shutdown(ctx context.Context) error {
	pm.mu.RLock()
	plugins := make([]plugin.Plugin, len(pm.plugins))
	copy(plugins, pm.plugins)
	pm.mu.RUnlock()

	for i := len(plugins) - 1; i >= 0; i-- {
		if err := plugins[i].Shutdown(ctx); err != nil {
			return err
		}
	}

	return nil
}

// ExecuteBeforeNode runs all plugins BeforeNode hooks.
// Returns (targets, updates, error) if any plugin short-circuits execution.
// Returns (nil, nil, nil) if all plugins allow normal execution.
func (pm *PluginManager) ExecuteBeforeNode(ctx context.Context, nodeName string, view state.ReadView) ([]string, state.Updates, error) {
	pm.mu.RLock()
	plugins := pm.plugins
	pm.mu.RUnlock()

	for _, p := range plugins {
		targets, updates, err := safeExecuteBeforeNode(ctx, p, nodeName, view)
		if err != nil {
			return nil, nil, err
		}
		// If plugin returns targets, it's short-circuiting execution
		if len(targets) > 0 {
			return targets, updates, nil
		}
	}

	return nil, nil, nil
}

// ExecuteAfterNode runs all plugins AfterNode hooks.
// Plugins can mutate the updates map to enrich or transform the node's output.
func (pm *PluginManager) ExecuteAfterNode(ctx context.Context, nodeName string, view state.ReadView, updates state.Updates) error {
	pm.mu.RLock()
	plugins := pm.plugins
	pm.mu.RUnlock()

	for _, p := range plugins {
		if err := safeExecuteAfterNode(ctx, p, nodeName, view, updates); err != nil {
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
func (pm *PluginManager) ExecuteAfterTool(ctx context.Context, toolName string, result any) error {
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

// ExecuteOnNodeError runs all plugins OnNodeError hooks.
func (pm *PluginManager) ExecuteOnNodeError(ctx context.Context, nodeName string, err error) error {
	pm.mu.RLock()
	plugins := pm.plugins
	pm.mu.RUnlock()

	for _, p := range plugins {
		if hookErr := safeExecuteOnNodeError(ctx, p, nodeName, err); hookErr != nil {
			return hookErr
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
func (pm *PluginManager) ExecuteOnStateChange(ctx context.Context, nodeName string, updates state.Updates) error {
	pm.mu.RLock()
	plugins := pm.plugins
	pm.mu.RUnlock()

	for _, p := range plugins {
		if err := safeExecuteOnStateChange(ctx, p, nodeName, updates); err != nil {
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
