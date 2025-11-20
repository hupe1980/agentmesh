package main

import (
	"context"
	"fmt"
	"iter"
	"log"
	"time"

	"github.com/hupe1980/agentmesh/pkg/agent"
	"github.com/hupe1980/agentmesh/pkg/callbacks"
	"github.com/hupe1980/agentmesh/pkg/callbacks/plugins"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	graphstate "github.com/hupe1980/agentmesh/pkg/state"
)

// FlakyModel simulates an unreliable external service
type FlakyModel struct {
	callCount int
}

func (m *FlakyModel) Capabilities() model.Capabilities {
	return model.Capabilities{
		Streaming:           false,
		Tools:               false,
		MaxContextTokens:    4096,
		MaxOutputTokens:     1024,
		SupportedModalities: []string{"text"},
	}
}

func (m *FlakyModel) Generate(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
	return func(yield func(*model.Response, error) bool) {
		m.callCount++

		// Simulate service behavior:
		// Calls 1-5: Fail (circuit opens after 3)
		// Calls 6+: Success (circuit recovers)
		if m.callCount <= 5 {
			log.Printf("[Call %d] ❌ Service failing", m.callCount)
			yield(nil, fmt.Errorf("service unavailable (call %d)", m.callCount))
			return
		}

		log.Printf("[Call %d] ✓ Service success", m.callCount)
		yield(&model.Response{
			Message: message.NewAIMessageFromText(fmt.Sprintf("Success on call %d", m.callCount)),
			Partial: false, // Single complete response
		}, nil)
	}
}

func main() {
	fmt.Println("=== Circuit Breaker Pattern Example ===")
	fmt.Println()
	fmt.Println("Demonstrating plugin-based circuit breaker:")
	fmt.Println("- First 3 failures → Circuit opens")
	fmt.Println("- While open, plugin rejects requests")
	fmt.Println("- After 5s timeout → Circuit transitions to half-open")
	fmt.Println("- Successful call → Circuit closes")
	fmt.Println()

	// Create a flaky model
	flakyModel := &FlakyModel{}

	// Create plugin manager with circuit breaker
	pluginMgr := callbacks.NewPluginManager()

	// Configure circuit breaker plugin:
	// - Opens after 3 failures
	// - Waits 5 seconds before transitioning to half-open
	// - Allows 1 test request in half-open state
	cbPlugin := plugins.NewCircuitBreakerPlugin(3, 5*time.Second, 1)

	if err := pluginMgr.Register(context.Background(), cbPlugin); err != nil {
		log.Fatal(err)
	}

	// Build the graph using agent
	mgr := graphstate.NewManager()
	graphstate.RegisterKey(mgr, agent.MessagesKey.Key)

	g, err := graph.NewGraph(mgr)
	if err != nil {
		log.Fatal(err)
	}

	modelNode, err := agent.NewModelNode(
		flakyModel,
		agent.WithModelNodeName("flaky-service"),
		agent.WithModelCallbacks(pluginMgr),
	)
	if err != nil {
		log.Fatal(err)
	}

	err = g.AddNode(modelNode)
	if err != nil {
		log.Fatal(err)
	}

	g.AddEdge(graph.StartNode, "flaky-service")
	g.AddEdge("flaky-service", graph.EndNode)

	compiled, err := graph.Compile(g, graph.NewMessagePregelExecutor())
	if err != nil {
		log.Fatal(err)
	}

	// Make multiple attempts to demonstrate circuit breaker behavior
	ctx := context.Background()
	var lastErr error

	for i := 1; i <= 10; i++ {
		fmt.Printf("\n--- Attempt %d ---\n", i)

		result, err := graph.Last(compiled.Run(ctx, []message.Message{
			message.NewHumanMessageFromText(fmt.Sprintf("Test attempt %d", i)),
		}))

		lastErr = err

		if err != nil {
			fmt.Printf("❌ Attempt failed: %v\n", err)
		} else if result != nil {
			text := message.Stringify(result)
			fmt.Printf("✓ Success: %s\n", text)
			break
		}

		// Wait before next attempt
		time.Sleep(time.Second)
	}

	fmt.Println("\n=== Results ===")
	fmt.Printf("Circuit breaker state: %v\n", cbPlugin.GetState())
	fmt.Printf("Total model calls made: %d\n", flakyModel.callCount)

	if lastErr != nil {
		fmt.Printf("❌ Final error: %v\n", lastErr)
	} else {
		fmt.Println("✓ Service recovered successfully!")
	}
}
