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

	graphstate "github.com/hupe1980/agentmesh/pkg/state"

	"github.com/hupe1980/agentmesh/pkg/exec"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
)

func main() {
	st := graphstate.NewState()
	graphstate.Register(st, graphstate.MessagesKey.Key)

	g, err := graph.NewGraph(st)
	if err != nil {
		panic(err)
	}

	// Create a simple echo node
	err = g.AddNode(&graph.Node{
		Name: "echo",
		RunFunc: func(ctx context.Context, view *graphstate.ReadView) (*graph.NodeResult, error) {
			messages := graphstate.GetMessages(view)
			lastMsg := messages[len(messages)-1].Message

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

	compiled, err := exec.CompileGraph(g)
	if err != nil {
		log.Fatal(err)
	}

	// Type assert to access State() method
	rg := compiled.(*exec.RunnableGraph)

	// Example 1: Unlimited messages (default)
	fmt.Println("=== With MaxMessages=0 (unlimited) ===")
	_, err = graph.Last(compiled.Run(context.Background(), []message.Message{
		message.NewHumanMessageFromText("Message 1"),
		message.NewHumanMessageFromText("Message 2"),
		message.NewHumanMessageFromText("Message 3"),
	}))
	if err != nil {
		log.Fatal(err)
	}
	snap1 := rg.State().Snapshot()
	messages1 := graphstate.GetMessages(graphstate.NewReadView(snap1))
	fmt.Printf("Messages retained: %d\n\n", len(messages1))

	// Example 2: Limit to 2 messages
	fmt.Println("=== With MaxMessages=2 ===")
	_, err = graph.Last(compiled.Run(context.Background(), []message.Message{
		message.NewHumanMessageFromText("Message 1"),
		message.NewHumanMessageFromText("Message 2"),
		message.NewHumanMessageFromText("Message 3"),
	}, graph.WithMaxMessages(2)))
	if err != nil {
		log.Fatal(err)
	}
	snap2 := rg.State().Snapshot()
	messages2 := graphstate.GetMessages(graphstate.NewReadView(snap2))
	fmt.Printf("Messages retained: %d (keeps most recent)\n", len(messages2))

	// Example 3: Recommended for long-running agents
	fmt.Println("\n=== Recommended Configuration ===")
	fmt.Println("For long-running agents, set WithMaxMessages(100-1000)")
	fmt.Println("This prevents OOM while retaining sufficient context")

	// Simulate long-running agent with retention
	_, err = graph.Last(compiled.Run(context.Background(), []message.Message{
		message.NewHumanMessageFromText("Start long conversation"),
	}, graph.WithMaxMessages(100)))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("✓ Agent executed with bounded memory")
}
