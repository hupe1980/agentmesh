// Package main demonstrates message retention and history management.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/hupe1980/agentmesh/pkg/graph"
)

// Use ListKey for message history
var messagesKey = graph.NewListKey[string]("messages")

func main() {
	ctx := context.Background()
	fmt.Println("=== Message Retention Example ===")

	// Build graph that accumulates messages
	g := graph.New[any, any](messagesKey)

	g.Node("user_input", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		fmt.Println("  [user_input] Adding user message")
		return graph.Append(messagesKey, "User: Hello, how are you?").To("assistant")
	}, "assistant")

	g.Node("assistant", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		messages := graph.GetList(scope, messagesKey)
		fmt.Printf("  [assistant] Current history: %d messages\n", len(messages))
		return graph.Append(messagesKey, "Assistant: I'm doing well, thanks!").To("followup")
	}, "followup")

	g.Node("followup", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		return graph.Append(messagesKey, "User: What can you help with?").To("response")
	}, "response")

	g.Node("response", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		return graph.Append(messagesKey, "Assistant: I can help with many things!").To("show_history")
	}, "show_history")

	g.Node("show_history", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		messages := graph.GetList(scope, messagesKey)
		fmt.Println("\n  Full conversation history:")
		for i, msg := range messages {
			fmt.Printf("    [%d] %s\n", i+1, msg)
		}
		return graph.Cmd().End()
	}, graph.END)

	g.Start("user_input")

	compiled, err := g.Build()
	if err != nil {
		log.Fatal(err)
	}

	for _, err := range compiled.Run(ctx, nil) {
		if err != nil {
			log.Fatal(err)
		}
	}

	fmt.Println("\n  Message retention enables:")
	fmt.Println("    • Conversation history for context")
	fmt.Println("    • Multi-turn dialogue")
	fmt.Println("    • Audit trails")
	fmt.Println("    • Token management (trim old messages)")
}
