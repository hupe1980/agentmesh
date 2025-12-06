package agent

import (
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/tool"
)

// This file defines common options that work across multiple agent types.
//
// The key design pattern here is using embedded structs to share common configuration.
// Both reActOptions and supervisorOptions embed commonOptions, which allows us to:
//
//  1. Define option functions once (WithSystemPrompt, WithMaxIterations, etc.)
//  2. Use the same functions with both ReAct and Supervisor agents
//  3. Avoid code duplication and naming conflicts
//
// The sharedOption type implements both ReActOption and SupervisorOption interfaces,
// allowing a single option function to work with any agent type that has embedded commonOptions.
//
// Example usage:
//
//	// Same option works for both agent types
//	reactAgent, _ := agent.NewReAct(model, agent.WithSystemPrompt("..."), agent.WithMaxIterations(10))
//	supervisor, _ := agent.NewSupervisor(model, agent.WithSystemPrompt("..."), agent.WithMaxIterations(10))

// commonOptions holds configuration shared across all agent types.
type commonOptions struct {
	systemPrompt    string
	maxIterations   int
	graphMiddleware []graph.Middleware
	modelMiddleware []model.Middleware
	toolMiddleware  []tool.Middleware
}

// SharedOption wraps a function that modifies commonOptions and implements
// both ReActOption and SupervisorOption interfaces.
//
// This allows common option functions (like WithSystemPrompt) to work with any agent
// type that embeds commonOptions, eliminating the need for type-prefixed functions
// like "WithSupervisorSystemPrompt" or "WithReActSystemPrompt".
type SharedOption func(*commonOptions)

// Implement ReActOption interface
func (s SharedOption) applyReAct(opts *reActOptions) {
	s(&opts.commonOptions)
}

// Implement SupervisorOption interface
func (s SharedOption) applySupervisor(opts *supervisorOptions) {
	s(&opts.commonOptions)
}

// Common option functions that work for both ReAct and Supervisor agents.
// They return sharedOption which can be converted to either option type.

// WithSystemPrompt sets the system prompt for any agent type.
func WithSystemPrompt(prompt string) SharedOption {
	return func(c *commonOptions) {
		c.systemPrompt = prompt
	}
}

// WithMaxIterations sets the maximum reasoning iterations for any agent type.
func WithMaxIterations(n int) SharedOption {
	return func(c *commonOptions) {
		if n > 0 {
			c.maxIterations = n
		}
	}
}

// WithGraphMiddleware adds middleware to the graph for any agent type.
func WithGraphMiddleware(middleware ...graph.Middleware) SharedOption {
	return func(c *commonOptions) {
		c.graphMiddleware = append(c.graphMiddleware, middleware...)
	}
}

// WithModelMiddleware adds middleware to the model executor for any agent type.
func WithModelMiddleware(middleware ...model.Middleware) SharedOption {
	return func(c *commonOptions) {
		c.modelMiddleware = append(c.modelMiddleware, middleware...)
	}
}

// WithToolMiddleware adds middleware to the tool executor for any agent type.
func WithToolMiddleware(middleware ...tool.Middleware) SharedOption {
	return func(c *commonOptions) {
		c.toolMiddleware = append(c.toolMiddleware, middleware...)
	}
}
