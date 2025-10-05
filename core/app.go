package core

// App represents a runnable application exposing its name, root agent, and plugins.
type App interface {
	// Name returns the application's identifier.
	Name() string

	// RootAgent returns the application's entry-point agent.
	RootAgent() Agent

	// Plugins returns the application's configured plugins.
	Plugins() []Plugin
}
