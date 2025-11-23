package integration_test

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/state"
)

// TestEarlyConsumerTermination verifies that stopping iteration early doesn't leak goroutines.
// This tests the fix for the iterator goroutine leak issue.
func TestEarlyConsumerTermination(t *testing.T) {
	ctx := context.Background()

	// Create a graph with many nodes to ensure runtime would continue if not cancelled
	sm := newTestManager()

	g, err := graph.NewGraph(sm)
	if err != nil {
		t.Fatalf("Failed to create graph: %v", err)
	}

	// Add multiple nodes that would take time to execute
	for i := 0; i < 10; i++ {
		nodeName := string(rune('A' + i))
		nextNode := string(rune('A' + i + 1))
		if i == 9 {
			nextNode = graph.EndNode
		}
		g.AddNode(&graph.BaseCommandNode{
			NodeName:        nodeName,
			DeclaredTargets: graph.NewTargetSet(nextNode),
			Fn: func(ctx context.Context, s state.ReadView) (*graph.Command, error) {
				// Simulate work
				time.Sleep(10 * time.Millisecond)
				return graph.GotoOne(nextNode), nil
			},
		})

		if i == 0 {
			g.SetEntryPoint(nodeName)
		}
	}
	// Compile the graph
	compiled, err := graph.Compile(g, graph.NewMessagePregelExecutor())
	if err != nil {
		t.Fatalf("Failed to compile graph: %v", err)
	}

	// Count goroutines before execution
	runtime.GC()
	time.Sleep(50 * time.Millisecond) // Let GC settle
	goroutinesBefore := runtime.NumGoroutine()

	// Run the graph but stop after first result (early termination)
	count := 0
	for result := range compiled.Run(ctx, []message.Message{message.NewHumanMessageFromText("start")}) {
		t.Logf("Got result: %v", result)
		count++
		if count >= 2 {
			// Stop early - this should cancel the runtime and not leak goroutines
			break
		}
	}

	// Give time for goroutines to clean up
	time.Sleep(100 * time.Millisecond)
	runtime.GC()
	time.Sleep(50 * time.Millisecond)

	// Count goroutines after execution
	goroutinesAfter := runtime.NumGoroutine()

	// There should not be a significant increase in goroutines
	// Allow some tolerance for background goroutines
	goroutineIncrease := goroutinesAfter - goroutinesBefore
	if goroutineIncrease > 5 {
		t.Errorf("Potential goroutine leak detected: before=%d, after=%d, increase=%d",
			goroutinesBefore, goroutinesAfter, goroutineIncrease)
	} else {
		t.Logf("✅ No goroutine leak: before=%d, after=%d, increase=%d",
			goroutinesBefore, goroutinesAfter, goroutineIncrease)
	}
}

// TestMultipleEarlyTerminations ensures the fix works across multiple runs
func TestMultipleEarlyTerminations(t *testing.T) {
	ctx := context.Background()

	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	goroutinesBefore := runtime.NumGoroutine()

	// Run multiple graphs with early termination
	for run := 0; run < 5; run++ {
		sm := newTestManager()
		g, _ := graph.NewGraph(sm)

		// Simple 3-node graph
		for i := 0; i < 3; i++ {
			nodeName := string(rune('A' + i))
			nextNode := string(rune('A' + i + 1))
			if i == 2 {
				nextNode = graph.EndNode
			}
			g.AddNode(&graph.BaseCommandNode{
				NodeName:        nodeName,
				DeclaredTargets: graph.NewTargetSet(nextNode),
				Fn: func(ctx context.Context, s state.ReadView) (*graph.Command, error) {
					time.Sleep(5 * time.Millisecond)
					return graph.GotoOne(nextNode), nil
				},
			})
			if i == 0 {
				g.SetEntryPoint(nodeName)

			} else {
			}
		}
		// Connect last node to end
		compiled, _ := graph.Compile(g, graph.NewMessagePregelExecutor())

		// Stop after first result
		for range compiled.Run(ctx, []message.Message{message.NewHumanMessageFromText("test")}) {
			break // Immediate early termination
		}
	} // Allow cleanup
	time.Sleep(200 * time.Millisecond)
	runtime.GC()
	time.Sleep(50 * time.Millisecond)

	goroutinesAfter := runtime.NumGoroutine()
	goroutineIncrease := goroutinesAfter - goroutinesBefore

	if goroutineIncrease > 10 {
		t.Errorf("Goroutine leak after multiple runs: before=%d, after=%d, increase=%d",
			goroutinesBefore, goroutinesAfter, goroutineIncrease)
	} else {
		t.Logf("✅ No leak across multiple runs: before=%d, after=%d, increase=%d",
			goroutinesBefore, goroutinesAfter, goroutineIncrease)
	}
}
