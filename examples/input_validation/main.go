package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	graphstate "github.com/hupe1980/agentmesh/pkg/state"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
)

func main() {
	fmt.Println("=== AgentMesh Input Validation Demo ===")
	fmt.Println("Demonstrates built-in message size and count limits for security")
	fmt.Println()

	// Create a simple graph
	builder, err := graph.NewBuilder()
	if err != nil {
		panic(err)
	}
	builder.
		Node("echo", func(ctx context.Context, s graphstate.Writer) (*graph.NodeResult, error) {
			events := s.MessagesSnapshot()
			if len(events) > 0 {
				fmt.Printf("Processing message: %s\n", ExtractText(events[len(events)-1].Message))
			}
			return &graph.NodeResult{}, nil
		}).
		AddEdge(graph.StartNode, "echo").
		AddEdge("echo", graph.EndNode)

	compiled, err := builder.Compile()
	if err != nil {
		log.Fatalf("Failed to compile graph: %v", err)
	}

	// Test 1: Normal message (should succeed)
	fmt.Println("Test 1: Normal message within limits")
	messages1 := []message.Message{
		message.NewHumanMessageFromText("Hello, world!"),
	}

	ctx := context.Background()
	_, err = graph.Last(compiled.Run(ctx, messages1,
		graph.WithMaxMessageSize(1_000_000), // 1MB per message
		graph.WithMaxInputMessages(100),     // Max 100 messages
		graph.WithMaxTotalSize(10_000_000),  // 10MB total
	))
	if err != nil {
		log.Printf("❌ Error: %v\n", err)
	} else {
		fmt.Println("✅ Succeeded")
	}
	fmt.Println()

	// Test 2: Message exceeds size limit (should fail)
	fmt.Println("Test 2: Single message exceeds size limit")
	hugeMessage := message.NewHumanMessageFromText(strings.Repeat("A", 2_000_000)) // 2MB
	messages2 := []message.Message{hugeMessage}

	_, err = graph.Last(compiled.Run(ctx, messages2,
		graph.WithMaxMessageSize(1_000_000), // 1MB limit
	))
	if err != nil {
		fmt.Printf("✅ Correctly blocked: %v\n", err)
	} else {
		fmt.Println("❌ Should have been blocked")
	}
	fmt.Println()

	// Test 3: Too many messages (should fail)
	fmt.Println("Test 3: Too many messages")
	manyMessages := make([]message.Message, 150)
	for i := range manyMessages {
		manyMessages[i] = message.NewHumanMessageFromText(fmt.Sprintf("Message %d", i))
	}

	_, err = graph.Last(compiled.Run(ctx, manyMessages,
		graph.WithMaxInputMessages(100), // Max 100 messages
	))
	if err != nil {
		fmt.Printf("✅ Correctly blocked: %v\n", err)
	} else {
		fmt.Println("❌ Should have been blocked")
	}
	fmt.Println()

	// Test 4: Total size exceeds limit (should fail)
	fmt.Println("Test 4: Total size of all messages exceeds limit")
	largeMessages := []message.Message{
		message.NewHumanMessageFromText(strings.Repeat("A", 400_000)),
		message.NewHumanMessageFromText(strings.Repeat("B", 400_000)),
		message.NewHumanMessageFromText(strings.Repeat("C", 400_000)),
	}

	_, err = graph.Last(compiled.Run(ctx, largeMessages,
		graph.WithMaxTotalSize(1_000_000), // 1MB total limit
	))
	if err != nil {
		fmt.Printf("✅ Correctly blocked: %v\n", err)
	} else {
		fmt.Println("❌ Should have been blocked")
	}
	fmt.Println()

	// Test 5: Production-recommended limits
	fmt.Println("Test 5: Production-recommended configuration")
	prodMessages := []message.Message{
		message.NewHumanMessageFromText("This is a normal user message"),
		message.NewHumanMessageFromText("Another reasonable message"),
	}

	_, err = graph.Last(compiled.Run(ctx, prodMessages,
		graph.WithMaxMessageSize(1_000_000), // 1MB per message
		graph.WithMaxInputMessages(100),     // Max 100 messages
		graph.WithMaxTotalSize(10_000_000),  // 10MB total
	))
	if err != nil {
		log.Printf("❌ Error: %v\n", err)
	} else {
		fmt.Println("✅ Production limits work correctly")
	}
	fmt.Println()

	fmt.Println("=== Summary ===")
	fmt.Println("Input validation protects against:")
	fmt.Println("- DoS attacks via extremely large messages")
	fmt.Println("- Resource exhaustion from too many messages")
	fmt.Println("- Excessive LLM API costs from bulk requests")
	fmt.Println()
	fmt.Println("Recommended production settings:")
	fmt.Println("- WithMaxMessageSize(1_000_000)   // 1MB per message")
	fmt.Println("- WithMaxInputMessages(100)       // Max 100 messages")
	fmt.Println("- WithMaxTotalSize(10_000_000)    // 10MB total")
}

// ExtractText is a helper to get text from a message
func ExtractText(msg message.Message) string {
	parts := msg.Parts()
	if len(parts) == 0 {
		return ""
	}
	if tp, ok := parts[0].(message.TextPart); ok {
		return tp.Text
	}
	return ""
}
