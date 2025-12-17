package agent

import (
	"fmt"

	"github.com/hupe1980/agentmesh/internal/validate"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/tool"
)

// WorkerAgent represents a specialized agent that can be supervised.
type WorkerAgent struct {
	Name        string       // Unique identifier for the worker
	Description string       // Description of the worker's expertise
	Agent       *graph.Graph // The agent to delegate work to
}

// supervisorOptions holds internal configuration for a supervisor agent.
type supervisorOptions struct {
	commonOptions
	workers        []WorkerAgent
	retryAttempts  int
	validateResult bool
}

// SupervisorOption configures a supervisor agent.
// It can be either a function or a sharedOption.
type SupervisorOption interface {
	applySupervisor(*supervisorOptions)
}

// supervisorOptionFunc wraps a function to implement SupervisorOption.
type supervisorOptionFunc func(*supervisorOptions)

func (f supervisorOptionFunc) applySupervisor(opts *supervisorOptions) {
	f(opts)
}

// WithWorker adds a worker agent to the supervisor.
// The agent must be a *graph.Graph (e.g., created via NewReAct).
func WithWorker(name, description string, agent *graph.Graph) SupervisorOption {
	return supervisorOptionFunc(func(c *supervisorOptions) {
		c.workers = append(c.workers, WorkerAgent{
			Name:        name,
			Description: description,
			Agent:       agent,
		})
	})
}

// WithWorkerRetries sets retry attempts for worker failures.
func WithWorkerRetries(attempts int) SupervisorOption {
	return supervisorOptionFunc(func(c *supervisorOptions) {
		c.retryAttempts = attempts
	})
}

// WithWorkerValidation enables validation of worker results.
func WithWorkerValidation(validate bool) SupervisorOption {
	return supervisorOptionFunc(func(c *supervisorOptions) {
		c.validateResult = validate
	})
}

// generateDefaultSupervisorInstructions creates default instructions based on available workers.
func generateDefaultSupervisorInstructions(workers []WorkerAgent) string {
	instructions := "You are a supervisor that routes questions to specialist agents.\n\n"
	instructions += "Available specialists:\n"

	for _, worker := range workers {
		instructions += fmt.Sprintf("- handoff_to_%s: %s\n", worker.Name, worker.Description)
	}

	instructions += "\nInstructions:\n"
	instructions += "- Analyze the user's question carefully\n"
	instructions += "- Delegate to the most appropriate specialist\n"
	instructions += "- Provide the full task context when delegating\n"
	instructions += "- Return the specialist's response directly to the user\n"

	return instructions
}

// NewSupervisor creates a supervisor agent that delegates work to specialized worker agents.
// The supervisor uses a model to decide which worker should handle each request.
//
// Returns a *graph.Graph that enables type-safe composition with other agents.
// Worker agents must also be *graph.Graph.
//
// Example:
//
//	supervisor, err := agent.NewSupervisor(
//	    model,
//	    agent.WithWorker("math", "Math expert", mathAgent),
//	    agent.WithWorker("code", "Programming expert", codeAgent),
//	    agent.WithInstructions("Route to specialists"),
//	    agent.WithWorkerRetries(2),
//	)
func NewSupervisor(mdl model.Model, opts ...SupervisorOption) (*graph.Graph, error) {
	if err := validate.NotNil(mdl, "model"); err != nil {
		return nil, err
	}

	config := supervisorOptions{
		commonOptions: commonOptions{
			instructions:    nil,
			maxIterations:   10,
			nodeMiddleware:  nil,
			runMiddleware:   nil,
			modelMiddleware: nil,
			toolMiddleware:  nil,
		},
		workers:       make([]WorkerAgent, 0),
		retryAttempts: 2,
	}

	for _, opt := range opts {
		opt.applySupervisor(&config)
	}

	if err := validate.NotEmptySlice(config.workers, "workers"); err != nil {
		return nil, fmt.Errorf("supervisor: %w", err)
	}

	// Create handoff tools for each worker
	handoffTools := make([]tool.Tool, 0, len(config.workers))

	for _, worker := range config.workers {
		if worker.Agent == nil {
			return nil, fmt.Errorf("supervisor: worker %q has nil agent", worker.Name)
		}

		// HandoffToAgent will add the "handoff_to_" prefix, so pass the worker name directly
		handoffTool, err := tool.HandoffToAgent(
			worker.Name,
			worker.Description,
			worker.Agent,
			tool.WithRetries(config.retryAttempts),
			tool.WithValidation(config.validateResult),
		)
		if err != nil {
			return nil, fmt.Errorf("supervisor: failed to create handoff tool for %q: %w", worker.Name, err)
		}

		handoffTools = append(handoffTools, handoffTool)
	}

	// Build supervisor agent options
	reactOpts := []ReActOption{
		WithTools(handoffTools...),
		WithMaxIterations(config.maxIterations),
		WithStreaming(config.streaming),
	}

	// Forward middleware from commonOptions
	if len(config.nodeMiddleware) > 0 {
		reactOpts = append(reactOpts, WithNodeMiddleware(config.nodeMiddleware...))
	}

	if len(config.runMiddleware) > 0 {
		reactOpts = append(reactOpts, WithRunMiddleware(config.runMiddleware...))
	}

	if len(config.modelMiddleware) > 0 {
		reactOpts = append(reactOpts, WithModelMiddleware(config.modelMiddleware...))
	}

	if len(config.toolMiddleware) > 0 {
		reactOpts = append(reactOpts, WithToolMiddleware(config.toolMiddleware...))
	}

	// Add instructions if provided, otherwise generate default
	if config.instructions != nil {
		reactOpts = append(reactOpts, reActOptionFunc(func(o *reActOptions) {
			o.instructions = config.instructions
		}))
	} else {
		// Generate default supervisor instructions
		defaultInstructions := generateDefaultSupervisorInstructions(config.workers)
		reactOpts = append(reactOpts, WithInstructions(defaultInstructions))
	}

	return NewReAct(mdl, reactOpts...)
}
