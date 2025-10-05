package app

import "github.com/hupe1980/agentmesh/core"

// Options configure an application constructed with New.
type Options struct {
	// Plugins to use for extending the runner's capabilities.
	Plugins []core.Plugin
}

// DefaultOptions provides the baseline application configuration.
var DefaultOptions = Options{
	Plugins: []core.Plugin{},
}

// App represents a runnable application with a root agent and configuration.
type App struct {
	name      string
	rootAgent core.Agent
	opts      Options
}

// New constructs a new App instance with optional configuration overrides.
func New(name string, rootAgent core.Agent, optFns ...func(o *Options)) *App {
	opts := DefaultOptions

	for _, fn := range optFns {
		fn(&opts)
	}

	return &App{name: name, rootAgent: rootAgent, opts: opts}
}

// Name returns the application's registered name.
func (a *App) Name() string {
	return a.name
}

// RootAgent returns the application's root agent.
func (a *App) RootAgent() core.Agent {
	return a.rootAgent
}

// Plugins returns the plugins configured for this application.
func (a *App) Plugins() []core.Plugin {
	return a.opts.Plugins
}

// Compile-time interface compliance check.
var _ core.App = (*App)(nil)
