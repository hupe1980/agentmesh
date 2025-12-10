package agent

import (
	"context"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/schema"
	"github.com/hupe1980/agentmesh/pkg/tool"
)

// This file defines common options that work across multiple agent types.
//
// The key design pattern here is using embedded structs to share common configuration.
// Both reActOptions and supervisorOptions embed commonOptions, which allows us to:
//
//  1. Define option functions once (WithInstructions, WithMaxIterations, etc.)
//  2. Use the same functions with both ReAct and Supervisor agents
//  3. Avoid code duplication and naming conflicts
//
// The sharedOption type implements both ReActOption and SupervisorOption interfaces,
// allowing a single option function to work with any agent type that has embedded commonOptions.
//
// Example usage:
//
//	// Same option works for both agent types
//	reactAgent, _ := agent.NewReAct(model, agent.WithInstructions("..."), agent.WithMaxIterations(10))
//	supervisor, _ := agent.NewSupervisor(model, agent.WithInstructions("..."), agent.WithMaxIterations(10))

// commonOptions holds configuration shared across all agent types.
type commonOptions struct {
	instructions    *Instructions // Dynamic instructions (supports templates and providers)
	maxIterations   int
	outputSchema    *schema.OutputSchema
	graphMiddleware []graph.Middleware
	modelMiddleware []model.Middleware
	toolMiddleware  []tool.Middleware
}

// SharedOption wraps a function that modifies commonOptions and implements
// both ReActOption and SupervisorOption interfaces.
//
// This allows common option functions (like WithInstructions) to work with any agent
// type that embeds commonOptions, eliminating the need for type-prefixed functions
// like "WithSupervisorInstructions" or "WithReActInstructions".
type SharedOption func(*commonOptions)

// Implement ReActOption interface
func (s SharedOption) applyReAct(opts *reActOptions) {
	s(&opts.commonOptions)
}

// Implement SupervisorOption interface
func (s SharedOption) applySupervisor(opts *supervisorOptions) {
	s(&opts.commonOptions)
}

// Implement RAGOption interface
func (s SharedOption) applyRAG(opts *ragOptions) {
	s(&opts.commonOptions)
}

// Common option functions that work for both ReAct and Supervisor agents.
// They return sharedOption which can be converted to either option type.

// WithInstructions sets instructions from a template string.
// Uses Go text/template syntax - placeholders like {{.userName}} are substituted from state.
//
// Example:
//
//	WithInstructions("You are helping {{.userName}}. Task: {{default \"general\" .task}}")
func WithInstructions(templateStr string) SharedOption {
	return func(c *commonOptions) {
		inst := NewInstructions(templateStr)
		c.instructions = &inst
	}
}

// WithInstructionsFunc sets instructions from a dynamic function.
// Use when instructions need complex logic or external data beyond template substitution.
//
// Example:
//
//	WithInstructionsFunc(func(ctx context.Context, view graph.View) (string, error) {
//	    user := graph.Get(view, UserKey)
//	    if user.IsPremium {
//	        return "You are a premium assistant...", nil
//	    }
//	    return "You are a helpful assistant...", nil
//	})
func WithInstructionsFunc(f func(context.Context, graph.View) (string, error)) SharedOption {
	return func(c *commonOptions) {
		inst := NewInstructionsFromFunc(f)
		c.instructions = &inst
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

// WithOutputSchema sets a structured output schema for any agent type.
// The schema constrains the model to generate valid JSON matching the schema.
func WithOutputSchema(outputSchema *schema.OutputSchema) SharedOption {
	return func(c *commonOptions) {
		c.outputSchema = outputSchema
	}
}
