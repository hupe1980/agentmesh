// Package main demonstrates conversation history management with message retention policies.
//
// This example shows how to:
//   - Limit message history to prevent out-of-memory issues
//   - Configure message retention with ListKey maxSize parameter
//   - Handle long-running agent conversations efficiently
//   - Automatically prune old messages while keeping recent context
//   - Prevent token limit exceeding when using LLMs
//
// Key concepts:
//   - state.NewListKey[message.Message]("__messages__", maxSize): Set maximum message history size
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

	"github.com/hupe1980/agentmesh/pkg/agent"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	graphstate "github.com/hupe1980/agentmesh/pkg/state"
)

func main() {
	// Example 1: Unlimited messages (default)
	fmt.Println("=== With Unlimited Messages (maxSize=0) ===")
	runExample(0)

	// Example 2: Limit to 2 messages
	fmt.Println("\n=== With MaxSize=2 ===")
	runExample(2)

	// Example 3: Recommended for long-running agents
	fmt.Println("\n=== Recommended Production Configuration ===")
	fmt.Println("For long-running agents, set maxSize to 100-1000")
	fmt.Println("This prevents OOM while retaining sufficient context")
	runExample(100)
}

func runExample(maxSize int) {
	// Create message key with specific retention limit
	messagesKey := graphstate.NewListKey[message.Message]("__messages__", maxSize)

	mgr := graphstate.NewManager()
	graphstate.RegisterListKey(mgr, messagesKey)

	g, err := graph.NewGraph(mgr)
	if err != nil {
		panic(err)
	}

	// Create a simple echo node
	err = g.AddNode(&graph.BaseCommandNode{
		NodeName:        "echo",
		DeclaredTargets: graph.NewTargetSet(graph.EndNode),
		Fn: func(ctx context.Context, view graphstate.ReadView) (*graph.Command, error) {
			messages := agent.GetMessages(view)
			lastMsg := messages[len(messages)-1]

			builder := graph.NewUpdate()
			graph.UpdateAppend(builder, agent.MessagesKey,
				message.Message(message.NewAIMessageFromText(fmt.Sprintf("Echo: %v", lastMsg.Parts()))),
			)

			updates, err := builder.Build()
			if err != nil {
				return nil, err
			}
			return graph.End(updates), nil
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	if err := g.SetEntryPoint("echo"); err != nil {
		panic(err)
	}

	compiled, err := graph.Compile(g, graph.NewMessagePregelExecutor())
	if err != nil {
		log.Fatal(err)
	}

	// Run with 3 messages
	_, err = graph.Last(compiled.Run(context.Background(), []message.Message{
		message.NewHumanMessageFromText("Message 1"),
		message.NewHumanMessageFromText("Message 2"),
		message.NewHumanMessageFromText("Message 3"),
	}))
	if err != nil {
		log.Fatal(err)
	}

	// Check how many messages were retained
	view, err := compiled.Manager().CreateReadView(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	// ListKey embeds Key[[]T], so we can use GetFromView
	messages := graphstate.GetFromView(view, messagesKey.Key)
	fmt.Printf("Messages retained: %d (maxSize=%d)\n", len(messages), maxSize)

	if maxSize > 0 {
		fmt.Println("✓ Older messages automatically pruned")
	} else {
		fmt.Println("✓ All messages retained (unlimited)")
	}
}
