package agent

import (
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/tool"
)

// WorkerAgent represents a specialized agent that can be supervised.
type WorkerAgent struct {
	Name        string          // Unique identifier for the worker
	Description string          // Description of the worker's expertise
	Agent       MessageRunnable // The agent to delegate work to
}

// supervisorOptions holds internal configuration for a supervisor agent.
type supervisorOptions struct {
	workers        []WorkerAgent
	systemPrompt   string
	maxIterations  int
	includeContext bool
	retryAttempts  int
	validateResult bool
}

// SupervisorOption configures a supervisor agent.
type SupervisorOption func(*supervisorOptions)

// WithWorker adds a worker agent to the supervisor.
// The agent must implement MessageRunnable (e.g., created via NewReActAgent).
func WithWorker(name, description string, agent MessageRunnable) SupervisorOption {
	return func(c *supervisorOptions) {
		c.workers = append(c.workers, WorkerAgent{
			Name:        name,
			Description: description,
			Agent:       agent,
		})
	}
}

// WithSupervisorSystemPrompt sets the supervisor's system prompt.
func WithSupervisorSystemPrompt(prompt string) SupervisorOption {
	return func(c *supervisorOptions) {
		c.systemPrompt = prompt
	}
}

// WithSupervisorMaxIterations sets the maximum iterations for the supervisor.
func WithSupervisorMaxIterations(n int) SupervisorOption {
	return func(c *supervisorOptions) {
		c.maxIterations = n
	}
}

// WithWorkerContext controls whether conversation context is passed to workers.
func WithWorkerContext(include bool) SupervisorOption {
	return func(c *supervisorOptions) {
		c.includeContext = include
	}
}

// WithWorkerRetries sets retry attempts for worker failures.
func WithWorkerRetries(attempts int) SupervisorOption {
	return func(c *supervisorOptions) {
		c.retryAttempts = attempts
	}
}

// WithWorkerValidation enables validation of worker results.
func WithWorkerValidation(validate bool) SupervisorOption {
	return func(c *supervisorOptions) {
		c.validateResult = validate
	}
}

// generateDefaultSupervisorPrompt creates a default system prompt based on available workers.
func generateDefaultSupervisorPrompt(workers []WorkerAgent) string {
	prompt := "You are a supervisor that routes questions to specialist agents.\n\n"
	prompt += "Available specialists:\n"

	for _, worker := range workers {
		prompt += fmt.Sprintf("- handoff_to_%s: %s\n", worker.Name, worker.Description)
	}

	prompt += "\nInstructions:\n"
	prompt += "- Analyze the user's question carefully\n"
	prompt += "- Delegate to the most appropriate specialist\n"
	prompt += "- Provide the full task context when delegating\n"
	prompt += "- Return the specialist's response directly to the user\n"

	return prompt
}

// NewSupervisorAgent creates a supervisor agent that delegates work to specialized worker agents.
// The supervisor uses a model to decide which worker should handle each request.
//
// Returns a MessageRunnable that enables type-safe composition with other agents.
// Worker agents must also implement MessageRunnable.
//
// Example:
//
//	supervisor, err := agent.NewSupervisorAgent(
//	    model,
//	    agent.WithWorker("math", "Math expert", mathAgent),
//	    agent.WithWorker("code", "Programming expert", codeAgent),
//	    agent.WithSupervisorSystemPrompt("Route to specialists"),
//	    agent.WithWorkerContext(false),
//	    agent.WithWorkerRetries(2),
//	)
func NewSupervisorAgent(mdl model.Model, opts ...SupervisorOption) (MessageRunnable, error) {
	if mdl == nil {
		return nil, fmt.Errorf("model must not be nil")
	}

	config := supervisorOptions{
		workers:        make([]WorkerAgent, 0),
		maxIterations:  10,
		includeContext: false,
		retryAttempts:  2,
	}

	for _, opt := range opts {
		opt(&config)
	}

	if len(config.workers) == 0 {
		return nil, fmt.Errorf("supervisor: at least one worker agent is required")
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
			tool.WithContext(config.includeContext),
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
	}

	// Add system prompt if provided
	if config.systemPrompt != "" {
		reactOpts = append(reactOpts, WithSystemPrompt(config.systemPrompt))
	} else {
		// Generate default system prompt
		defaultPrompt := generateDefaultSupervisorPrompt(config.workers)
		reactOpts = append(reactOpts, WithSystemPrompt(defaultPrompt))
	}

	return NewReActAgent(mdl, reactOpts...)
}
