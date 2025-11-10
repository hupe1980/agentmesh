// Package main demonstrates conversation history management with message retention policies.
//
// This example shows how to:
//   - Limit message history to prevent out-of-memory issues
//   - Configure message retention with WithMaxMessages option
//   - Handle long-running agent conversations efficiently
//   - Automatically prune old messages while keeping recent context
//   - Prevent token limit exceeding when using LLMs
//
// Key concepts:
//   - NewGraphState(maxMessages): Set maximum message history size
//   - Automatic pruning: Old messages removed when limit exceeded
//   - Memory efficiency: Prevent OOM in long conversations
//   - Context window management: Keep relevant conversation history
//
// Use cases:
//   - Chat applications with many turns
//   - Long-running agent workflows
//   - Production deployments with memory constraints
//   - LLM context window management
//
// Run: go run main.go

package main

import (
	"context"
	"fmt"
	"log"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
)

func main() {
	state := graph.NewStateManager(0)
	g := graph.NewGraph(state)

	// Create a simple echo node
	err := g.AddNode(&graph.Node{
		Name: "echo",
		RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
			events := s.MessageEventsSnapshot()
			lastMsg := events[len(events)-1].Message

			return &graph.NodeResult{
				Messages: []message.Message{
					message.NewAIMessageFromText(fmt.Sprintf("Echo: %v", lastMsg.Parts())),
				},
			}, nil
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	g.AddEdge(graph.StartNode, "echo")
	g.AddEdge("echo", graph.EndNode)

	compiled, err := g.Compile()
	if err != nil {
		log.Fatal(err)
	}

	// Example 1: Unlimited retention (default)
	fmt.Println("=== Unlimited Retention ===")
	result1, err := compiled.Invoke(context.Background(), []message.Message{
		message.NewHumanMessageFromText("Message 1"),
		message.NewHumanMessageFromText("Message 2"),
		message.NewHumanMessageFromText("Message 3"),
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Messages retained: %d\n\n", len(result1))

	// Example 2: Limit to 2 messages
	fmt.Println("=== With MaxMessages=2 ===")
	result2, err := compiled.Invoke(context.Background(), []message.Message{
		message.NewHumanMessageFromText("Message 1"),
		message.NewHumanMessageFromText("Message 2"),
		message.NewHumanMessageFromText("Message 3"),
	}, graph.WithMaxMessages(2))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Messages retained: %d (keeps most recent)\n", len(result2))

	// Example 3: Recommended for long-running agents
	fmt.Println("\n=== Recommended Configuration ===")
	fmt.Println("For long-running agents, set WithMaxMessages(100-1000)")
	fmt.Println("This prevents OOM while retaining sufficient context")

	// Simulate long-running agent with retention
	_, err = compiled.Invoke(context.Background(), []message.Message{
		message.NewHumanMessageFromText("Start long conversation"),
	}, graph.WithMaxMessages(100))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("✓ Agent executed with bounded memory")
}
