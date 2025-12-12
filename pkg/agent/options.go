package agent

import (
	"context"

	"github.com/hupe1980/agentmesh/pkg/guardrail"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	modelmw "github.com/hupe1980/agentmesh/pkg/model/middleware"
	"github.com/hupe1980/agentmesh/pkg/schema"
	"github.com/hupe1980/agentmesh/pkg/tool"
	toolmw "github.com/hupe1980/agentmesh/pkg/tool/middleware"
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
	streaming       bool                     // Enable streaming mode for real-time output
	nodeMiddleware  []message.NodeMiddleware // Node-level middleware (wraps each node)
	runMiddleware   []message.RunMiddleware  // Run-level middleware (wraps Run/Resume)
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
//	WithInstructionsFunc(func(ctx context.Context, scope message.Scope) (string, error) {
//	    user := graph.Get(scope, UserKey)
//	    if user.IsPremium {
//	        return "You are a premium assistant...", nil
//	    }
//	    return "You are a helpful assistant...", nil
//	})
func WithInstructionsFunc(f func(context.Context, message.Scope) (string, error)) SharedOption {
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

// WithNodeMiddleware adds node-level middleware to the graph for any agent type.
// Node middleware wraps each node execution and runs for every node.
// For middleware that should wrap the entire Run/Resume operation, use WithRunMiddleware.
func WithNodeMiddleware(middleware ...message.NodeMiddleware) SharedOption {
	return func(c *commonOptions) {
		c.nodeMiddleware = append(c.nodeMiddleware, middleware...)
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

// WithRunMiddleware adds run-level middleware to the agent's graph.
// Run middleware wraps the entire Run/Resume operation, intercepting:
//   - Input before execution starts
//   - Output after execution completes
//
// This is useful for:
//   - Input validation/guardrails (check user input once at start)
//   - Output validation/guardrails (check final output once at end)
//   - Logging/observability at the run level
//   - Request/response transformation
//
// Middleware is applied in order: first added = outermost wrapper.
//
// Example:
//
//	agent.NewReAct(model,
//	    agent.WithRunMiddleware(
//	        agent.InputGuardrailMiddleware(myInputGuardrail),
//	        agent.OutputGuardrailMiddleware(myOutputGuardrail),
//	    ),
//	)
func WithRunMiddleware(middleware ...message.RunMiddleware) SharedOption {
	return func(c *commonOptions) {
		c.runMiddleware = append(c.runMiddleware, middleware...)
	}
}

// WithOutputSchema sets a structured output schema for any agent type.
// The schema constrains the model to generate valid JSON matching the schema.
func WithOutputSchema(outputSchema *schema.OutputSchema) SharedOption {
	return func(c *commonOptions) {
		c.outputSchema = outputSchema
	}
}

// WithStreaming enables streaming mode for real-time output.
// When enabled, partial responses are streamed via the graph's stream writer,
// allowing real-time display of AI responses as they're generated.
//
// To receive streamed values, provide a stream handler when running the graph:
//
//	graph.WithStreamHandler(func(msg message.Message) {
//	    fmt.Print(msg.Text())
//	})
func WithStreaming(enabled bool) SharedOption {
	return func(c *commonOptions) {
		c.streaming = enabled
	}
}

// -----------------------------------------------------------------------------
// Guardrail Options
// -----------------------------------------------------------------------------
//
// AgentMesh provides guardrails at three different levels:
//
// 1. Model-level: Runs on EVERY LLM call (use for content filtering)
//    - WithModelInputGuardrails: validates input before each model call
//    - WithModelOutputGuardrails: validates output after each model response
//
// 2. Graph-level: Runs ONCE per graph execution (use for request/response validation)
//    - WithGraphInputGuardrails: validates user input once at start
//    - WithGraphOutputGuardrails: validates final output once at end
//
// 3. Tool-level: Runs on EVERY tool call (use for tool argument/result validation)
//    - WithToolInputGuardrails: validates tool arguments before execution
//    - WithToolOutputGuardrails: validates tool results after execution

// ModelInputGuardrailConfig configures model input guardrail behavior.
type ModelInputGuardrailConfig struct {
	// Parallel controls whether guardrails run concurrently with model execution.
	// When true: guardrails run in parallel with model - better latency but model may
	// consume tokens before guardrail completes.
	// When false (default): guardrails complete before model starts - prevents token consumption.
	Parallel bool
}

// WithModelInputGuardrails adds input guardrails as model middleware for agents.
// These guardrails run on EVERY LLM call to validate input content.
// By default, guardrails run in blocking mode (complete before model starts).
//
// For parallel execution (better latency, but model may consume tokens):
//
//	agent.WithModelInputGuardrails(guardrails, agent.ModelInputGuardrailConfig{Parallel: true})
//
// For guardrails that should only check the initial user input once,
// use WithGraphInputGuardrails instead.
func WithModelInputGuardrails(guardrails []guardrail.Guardrail[string], config ...ModelInputGuardrailConfig) SharedOption {
	return func(c *commonOptions) {
		opts := []modelmw.GuardrailOption{
			modelmw.WithInputGuardrails(guardrails...),
		}
		if len(config) > 0 && config[0].Parallel {
			opts = append(opts, modelmw.WithInputParallel(true))
		}
		c.modelMiddleware = append(c.modelMiddleware, modelmw.NewGuardrailMiddleware(opts...))
	}
}

// WithModelOutputGuardrails adds output guardrails as model middleware for agents.
// These guardrails run on EVERY LLM response to validate output content.
//
// For guardrails that should only check the final output once,
// use WithGraphOutputGuardrails instead.
func WithModelOutputGuardrails(guardrails ...guardrail.Guardrail[string]) SharedOption {
	return func(c *commonOptions) {
		mw := modelmw.NewGuardrailMiddleware(
			modelmw.WithOutputGuardrails(guardrails...),
		)
		c.modelMiddleware = append(c.modelMiddleware, mw)
	}
}

// WithGraphInputGuardrails adds input guardrails that run ONCE at the start of graph execution.
// Use this when you want to validate the user's initial input before any processing begins.
//
// This is a convenience wrapper for:
//
//	agent.WithRunMiddleware(agent.InputGuardrailMiddleware(guardrails...))
func WithGraphInputGuardrails(guardrails ...guardrail.Guardrail[string]) SharedOption {
	return func(c *commonOptions) {
		// Convert string guardrails to MessageInputGuardrails
		msgGuardrails := make([]MessageInputGuardrail, len(guardrails))
		for i, g := range guardrails {
			msgGuardrails[i] = NewMessageInputGuardrail(g)
		}
		c.runMiddleware = append(c.runMiddleware, InputGuardrailMiddleware(msgGuardrails...))
	}
}

// WithGraphOutputGuardrails adds output guardrails that run ONCE at the end of graph execution.
// Use this when you want to validate the final response before it's returned to the user.
//
// This is a convenience wrapper for:
//
//	agent.WithRunMiddleware(agent.OutputGuardrailMiddleware(guardrails...))
func WithGraphOutputGuardrails(guardrails ...guardrail.Guardrail[string]) SharedOption {
	return func(c *commonOptions) {
		// Convert string guardrails to MessageOutputGuardrails
		msgGuardrails := make([]MessageOutputGuardrail, len(guardrails))
		for i, g := range guardrails {
			msgGuardrails[i] = NewMessageOutputGuardrail(g)
		}
		c.runMiddleware = append(c.runMiddleware, OutputGuardrailMiddleware(msgGuardrails...))
	}
}

// WithToolInputGuardrails adds input guardrails that run on EVERY tool call.
// Use this to validate tool arguments before tool execution.
func WithToolInputGuardrails(guardrails ...guardrail.Guardrail[string]) SharedOption {
	return func(c *commonOptions) {
		mw := toolmw.NewGuardrailMiddleware(
			toolmw.WithInputGuardrails(guardrails...),
		)
		c.toolMiddleware = append(c.toolMiddleware, mw)
	}
}

// WithToolOutputGuardrails adds output guardrails that run on EVERY tool call.
// Use this to validate tool results after tool execution.
func WithToolOutputGuardrails(guardrails ...guardrail.Guardrail[string]) SharedOption {
	return func(c *commonOptions) {
		mw := toolmw.NewGuardrailMiddleware(
			toolmw.WithOutputGuardrails(guardrails...),
		)
		c.toolMiddleware = append(c.toolMiddleware, mw)
	}
}
