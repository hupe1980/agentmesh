package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/hupe1980/agentmesh/internal/validate"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
)

// State keys for reflection
var (
	// ReflectionCountKey tracks the number of reflections performed
	ReflectionCountKey = graph.NewKey[int]("reflection_count")
	// DraftKey stores the current draft answer being refined
	DraftKey = graph.NewKey[string]("draft")
)

// NewReflection creates a reflection agent that wraps another agent and adds
// self-critique and refinement capabilities. The reflection agent will:
//  1. Run the wrapped agent to get an initial answer
//  2. Critique the answer using a reflection model
//  3. Pass the critique back to the agent for refinement
//  4. Repeat until max reflections or quality threshold met
//
// This pattern allows any agent (ReAct, RAG, Supervisor, custom) to benefit
// from iterative self-improvement through reflection.
//
// Example wrapping a ReAct agent:
//
//	reactAgent, _ := agent.NewReAct(model, agent.WithTools(searchTool))
//	reflectionAgent, _ := agent.NewReflection(reactAgent, reflectionModel,
//	    agent.WithMaxReflections(3),
//	    agent.WithReflectionPrompt("Critique this answer..."))
//
// Example wrapping a RAG agent:
//
//	ragAgent, _ := agent.NewRAG(model, retriever)
//	reflectionAgent, _ := agent.NewReflection(ragAgent, reflectionModel,
//	    agent.WithMaxReflections(2))
func NewReflection(
	wrappedAgent *graph.Graph,
	reflectionModel model.Model,
	opts ...ReflectionOption,
) (*graph.Graph, error) {
	if err := validate.NotNil(wrappedAgent, "wrappedAgent"); err != nil {
		return nil, err
	}
	if err := validate.NotNil(reflectionModel, "reflectionModel"); err != nil {
		return nil, err
	}

	config := defaultReflectionOptions()
	for _, opt := range opts {
		opt.applyReflection(&config)
	}

	// Create reflection executor
	reflectionExecutor := model.NewExecutor(reflectionModel, model.WithExecutorName("reflection"))
	if len(config.modelMiddleware) > 0 {
		reflectionExecutor = model.Chain(reflectionExecutor, config.modelMiddleware...)
	}

	// Create agent node (wraps the inner agent)
	agentNode := createAgentWrapperNode(wrappedAgent)

	// Create reflection node
	reflectionNode := createReflectionNodeForWrapper(reflectionExecutor, config)

	// Build reflection graph
	return buildReflectionGraph(agentNode, reflectionNode, config)
}

// buildReflectionGraph constructs the reflection agent graph.
func buildReflectionGraph(agentNode, reflectionNode graph.NodeFunc, config reflectionOptions) (*graph.Graph, error) {
	b := graph.New()

	// Graph structure:
	// START → agent → [reflection | END]
	//           ↑         ↓
	//           └─────────┘
	b.Node("agent", agentNode, "reflection", graph.END)
	b.Node("reflection", reflectionNode, "agent")
	b.Start("agent")

	// Apply graph middleware if provided
	if len(config.graphMiddleware) > 0 {
		b.WithNodeMiddleware(config.graphMiddleware...)
	}

	return b.Build()
}

// reflectionOptions holds configuration for reflection agents.
type reflectionOptions struct {
	maxReflections      int
	reflectionPrompt    string
	reflectionThreshold float64
	graphMiddleware     []graph.NodeMiddleware
	modelMiddleware     []model.Middleware
}

func defaultReflectionOptions() reflectionOptions {
	return reflectionOptions{
		maxReflections:      3,
		reflectionPrompt:    defaultReflectionPrompt(),
		reflectionThreshold: 0.0,
		graphMiddleware:     nil,
		modelMiddleware:     nil,
	}
}

func defaultReflectionPrompt() string {
	return `Review the draft answer below and provide constructive critique.

Draft Answer:
{draft}

Analyze:
1. Is the answer accurate and complete?
2. Are there any logical errors or inconsistencies?
3. Could the explanation be clearer or more concise?
4. What specific improvements would make this answer better?

Provide your critique and suggestions for improvement.`
}

// ReflectionOption configures a Reflection agent.
type ReflectionOption interface {
	applyReflection(*reflectionOptions)
}

// reflectionOptionFunc wraps a function to implement ReflectionOption.
type reflectionOptionFunc func(*reflectionOptions)

func (f reflectionOptionFunc) applyReflection(opts *reflectionOptions) {
	f(opts)
}

// WithReflectionMaxIterations sets the maximum number of reflection iterations.
func WithReflectionMaxIterations(n int) ReflectionOption {
	return reflectionOptionFunc(func(c *reflectionOptions) {
		if n > 0 {
			c.maxReflections = n
		}
	})
}

// WithReflectionPromptTemplate sets the prompt used to critique answers.
// Use {draft} as placeholder for the answer to critique.
func WithReflectionPromptTemplate(prompt string) ReflectionOption {
	return reflectionOptionFunc(func(c *reflectionOptions) {
		c.reflectionPrompt = prompt
	})
}

// WithReflectionGraphMiddleware adds node middleware to the reflection graph.
func WithReflectionGraphMiddleware(middleware ...graph.NodeMiddleware) ReflectionOption {
	return reflectionOptionFunc(func(c *reflectionOptions) {
		c.graphMiddleware = append(c.graphMiddleware, middleware...)
	})
}

// WithReflectionModelMiddleware adds middleware to the reflection model executor.
func WithReflectionModelMiddleware(middleware ...model.Middleware) ReflectionOption {
	return reflectionOptionFunc(func(c *reflectionOptions) {
		c.modelMiddleware = append(c.modelMiddleware, middleware...)
	})
}

// createAgentWrapperNode wraps an agent graph as a node function.
func createAgentWrapperNode(wrappedAgent *graph.Graph) graph.NodeFunc {
	return func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		// Get current messages from state
		messages := scope.Messages()
		reflectionCount := graph.Get(scope, ReflectionCountKey)

		// Run the wrapped agent with current messages
		lastMsg, err := graph.Last(wrappedAgent.Run(ctx, messages))
		if err != nil {
			return graph.Fail(err)
		}

		// Append agent's answer to messages
		// Check if we should continue reflecting
		if reflectionCount > 0 {
			// This is a refinement iteration, route back to reflection
			return graph.Reply(lastMsg).To("reflection")
		}

		// First iteration, route to reflection
		return graph.Reply(lastMsg).To("reflection")
	}
}

// createReflectionNodeForWrapper creates reflection node for the wrapper pattern.
func createReflectionNodeForWrapper(executor model.Executor, config reflectionOptions) graph.NodeFunc {
	return func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		messages := scope.Messages()
		reflectionCount := graph.Get(scope, ReflectionCountKey)

		// Check if we've exceeded max reflections
		if reflectionCount >= config.maxReflections {
			// Stop reflecting, route to END
			return graph.To(graph.END)
		}

		// Extract the last AI message as the draft to critique
		var draft string
		for i := len(messages) - 1; i >= 0; i-- {
			if aiMsg, ok := messages[i].(*message.AIMessage); ok {
				draft = aiMsg.String()
				break
			}
		}

		if draft == "" {
			// No draft to reflect on, route to END
			return graph.To(graph.END)
		}

		// Build reflection prompt
		reflectionPrompt := strings.ReplaceAll(config.reflectionPrompt, "{draft}", draft)

		// Create reflection request
		reflectionMessages := []message.Message{
			message.NewSystemMessageFromText(reflectionPrompt),
			message.NewHumanMessageFromText("Please provide your critique and suggestions for improvement."),
		}

		req := &model.Request{
			Messages: reflectionMessages,
		}

		// Execute reflection
		resp, err := model.Last(executor.Generate(ctx, req))
		if err != nil {
			return graph.Fail(err)
		}

		// Add reflection as a system message to guide the next iteration
		reflectionText := resp.Message.String()
		reflectionMsgText := fmt.Sprintf("Reflection on your previous answer:\n%s\n\nPlease provide an improved answer based on this feedback.", reflectionText)
		var reflectionMsg message.Message = message.NewSystemMessageFromText(reflectionMsgText)

		// Increment reflection count and add reflection message
		return graph.Reply(reflectionMsg).
			With(graph.SetValue(ReflectionCountKey, reflectionCount+1)).
			With(graph.SetValue(DraftKey, draft)).
			To("agent")
	}
}
