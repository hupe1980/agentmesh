package callbacks

import "context"

type contextKey string

const pluginManagerKey contextKey = "plugin_manager"

// WithPluginManager adds a PluginManager to the context.
func WithPluginManager(ctx context.Context, pm *PluginManager) context.Context {
	return context.WithValue(ctx, pluginManagerKey, pm)
}

// PluginManagerFromContext retrieves the PluginManager from context.
// Returns the PluginManager and true if found, nil and false otherwise.
func PluginManagerFromContext(ctx context.Context) (*PluginManager, bool) {
	pm, ok := ctx.Value(pluginManagerKey).(*PluginManager)
	return pm, ok
}

// FromContext retrieves the PluginManager from context, returning nil if not found.
// This is a convenience wrapper around PluginManagerFromContext.
func FromContext(ctx context.Context) *PluginManager {
	pm, _ := PluginManagerFromContext(ctx)
	return pm
}
