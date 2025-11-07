package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/hupe1980/agentmesh/pkg/agent"
	"github.com/hupe1980/agentmesh/pkg/callbacks"
	"github.com/hupe1980/agentmesh/pkg/callbacks/policies"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
)

// FlakyModel simulates an unreliable external service
type FlakyModel struct {
	callCount int
}

func (m *FlakyModel) Generate(ctx context.Context, messages []message.Message) (message.Message, error) {
	m.callCount++

	// Simulate service behavior:
	// Calls 1-5: Fail (circuit opens after 3)
	// Calls 6+: Success (circuit recovers)
	if m.callCount <= 5 {
		log.Printf("[Call %d] ❌ Service failing", m.callCount)
		return nil, fmt.Errorf("service unavailable (call %d)", m.callCount)
	}

	log.Printf("[Call %d] ✓ Service success", m.callCount)
	return message.NewAIMessageFromText(fmt.Sprintf("Success on call %d", m.callCount)), nil
}

func (m *FlakyModel) Stream(ctx context.Context, messages []message.Message) (*model.Stream, error) {
	// Streaming not implemented for this example - just return message as single chunk
	msg, err := m.Generate(ctx, messages)
	if err != nil {
		return nil, err
	}

	chunks := make(chan model.StreamChunk, 1)
	chunks <- model.StreamChunk{Message: msg}
	close(chunks)

	return model.NewStream(chunks, nil), nil
}

func main() {
	fmt.Println("=== Circuit Breaker Pattern Example ===")
	fmt.Println()
	fmt.Println("Demonstrating callback-based circuit breaker:")
	fmt.Println("- First 3 failures → Circuit opens")
	fmt.Println("- While open, callbacks reject requests")
	fmt.Println("- After 5s timeout → Circuit transitions to half-open")
	fmt.Println("- Successful call → Circuit closes")
	fmt.Println()

	// Create a flaky model
	flakyModel := &FlakyModel{}

	// Create callback manager with circuit breaker
	manager := callbacks.NewManager()

	// Configure circuit breaker:
	// - Opens after 3 failures
	// - Waits 5 seconds before transitioning to half-open
	// - Tracks failures within a 1 minute window
	config := policies.DefaultCircuitBreakerConfig()
	config.MaxFailures = 3
	config.Timeout = 5 * time.Second
	config.FailureWindow = 1 * time.Minute

	before, after, onError := policies.CircuitBreaker(config)
	manager.RegisterBeforeModel(before)
	manager.RegisterAfterModel(after)
	manager.RegisterOnModelError(onError)

	// Add retry policy with short delays to see circuit breaker in action
	retryConfig := policies.DefaultRetryConfig()
	retryConfig.MaxAttempts = 20
	retryConfig.InitialDelay = 200 * time.Millisecond
	retryConfig.MaxDelay = 1 * time.Second
	manager.RegisterOnModelError(policies.ExponentialBackoffRetry(retryConfig))

	// Build the graph using agent
	state := graph.NewGraphState(10)
	g := graph.NewGraph(state)

	err := g.AddNode(agent.ModelNode(
		flakyModel,
		agent.WithModelNodeName("flaky-service"),
		agent.WithModelCallbacks(manager),
	))
	if err != nil {
		log.Fatal(err)
	}

	g.AddEdge(graph.StartNode, "flaky-service")
	g.AddEdge("flaky-service", graph.EndNode)

	compiled, err := g.Compile()
	if err != nil {
		log.Fatal(err)
	}

	result, err := compiled.Invoke(context.Background(), []message.Message{
		message.NewHumanMessageFromText("Test circuit breaker"),
	})

	fmt.Println("\n=== Results ===")
	if err != nil {
		fmt.Printf("❌ Final error: %v\n", err)
		fmt.Printf("Total calls attempted: %d\n", flakyModel.callCount)
	} else {
		fmt.Println("✓ Service recovered successfully!")
		fmt.Printf("Total calls made: %d\n", flakyModel.callCount)
		if len(result) > 0 {
			lastMsg := result[len(result)-1]
			parts := lastMsg.Parts()
			if len(parts) > 0 {
				if textPart, ok := parts[0].(message.TextPart); ok {
					fmt.Printf("Final response: %s\n", textPart.Text)
				}
			}
		}
	}
}
