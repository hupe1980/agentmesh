// Package main demonstrates real-time streaming execution in AgentMesh.
//
// This example shows how to:
//   - Use Stream for live updates during graph execution
//   - Stream partial results as nodes complete
//   - Track execution progress with real-time result handling
//   - Stream LLM responses token-by-token for better UX
//   - Handle cleanup with defer stream.Close() to prevent goroutine leaks
//
// Key concepts:
//   - Stream: Real-time execution result channel for graph execution
//   - Execution Results: NodeStart, NodeComplete, NodeError, GraphComplete
//   - Proper cleanup: Always call stream.Close() or stream.Cancel()
//
// Prerequisites:
//   export OPENAI_API_KEY="sk-..."
//
// Run: go run main.go

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	graphstate "github.com/hupe1980/agentmesh/pkg/state"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	pkgmodel "github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/model/openai"
	"github.com/logrusorgru/aurora/v3"
)

func main() {
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		log.Fatalf("OPENAI_API_KEY not set")
	}

	model := openai.NewModel()

	// Build a multi-node graph to demonstrate streaming
	builder, err := graph.NewBuilder()
	if err != nil {
		log.Fatalf("Failed to create builder: %v", err)
	}

	// Node 1: Data processor with intermediate streaming
	builder.Node("data_processor", func(ctx context.Context, s graphstate.Writer) (*graph.NodeResult, error) {
		// Get the stream writer to emit intermediate results
		streamWriter := graph.GetStreamWriter(ctx)

		fmt.Println("   ⏳ Processing data in chunks...")

		// Simulate processing multiple chunks and streaming progress
		chunks := []string{"chunk1", "chunk2", "chunk3", "chunk4"}
		for i, chunk := range chunks {
			time.Sleep(300 * time.Millisecond) // Simulate processing time

			// Emit intermediate progress via stream
			if streamWriter != nil {
				streamWriter(&graph.NodeResult{
					Updates: map[string]any{
						"progress":      fmt.Sprintf("%d/%d", i+1, len(chunks)),
						"current_chunk": chunk,
					},
				})
			}
		}

		return &graph.NodeResult{
			Updates: map[string]any{
				"status":       "data_processed",
				"chunks_total": len(chunks),
			},
		}, nil
	})

	// Node 2: LLM call with streaming
	builder.Node("llm_call", func(ctx context.Context, s graphstate.Writer) (*graph.NodeResult, error) {
		streamWriter := graph.GetStreamWriter(ctx)

		fmt.Println("   ⏳ Calling LLM...")

		// Emit pre-call status
		if streamWriter != nil {
			streamWriter(&graph.NodeResult{
				Updates: map[string]any{
					"llm_status": "starting",
				},
			})
		}

		// Get messages from state
		events := s.MessagesSnapshot()
		msgs := graphstate.ExtractMessages(events)

		// Create request
		req := &pkgmodel.Request{
			Messages: msgs,
		} // Call the model
		resp, err := pkgmodel.Last(model.Generate(ctx, req))
		if err != nil {
			return nil, err
		}

		// Emit post-call status
		if streamWriter != nil {
			streamWriter(&graph.NodeResult{
				Updates: map[string]any{
					"llm_status": "completed",
				},
			})
		}

		return &graph.NodeResult{
			Messages: []message.Message{resp.Message},
			Updates: map[string]any{
				"status": "llm_completed",
			},
		}, nil
	}) // Node 3: Multi-step analyzer with detailed streaming
	builder.Node("analyzer", func(ctx context.Context, s graphstate.Writer) (*graph.NodeResult, error) {
		streamWriter := graph.GetStreamWriter(ctx)

		fmt.Println("   ⏳ Analyzing results...")

		// Step 1: Validation
		time.Sleep(300 * time.Millisecond)
		if streamWriter != nil {
			streamWriter(&graph.NodeResult{
				Updates: map[string]any{
					"analysis_step": "validation",
					"validation":    "passed",
				},
			})
		}

		// Step 2: Quality check
		time.Sleep(300 * time.Millisecond)
		if streamWriter != nil {
			streamWriter(&graph.NodeResult{
				Updates: map[string]any{
					"analysis_step": "quality_check",
					"quality_score": 0.95,
				},
			})
		}

		// Step 3: Finalization
		time.Sleep(300 * time.Millisecond)
		if streamWriter != nil {
			streamWriter(&graph.NodeResult{
				Updates: map[string]any{
					"analysis_step": "finalization",
					"ready":         true,
				},
			})
		}

		return &graph.NodeResult{
			Updates: map[string]any{
				"status":   "analysis_complete",
				"verified": true,
			},
		}, nil
	})

	// Define the graph flow
	builder.AddEdge(graph.StartNode, "data_processor")
	builder.AddEdge("data_processor", "llm_call")
	builder.AddEdge("llm_call", "analyzer")
	builder.AddEdge("analyzer", graph.EndNode)

	compiled, err := builder.Compile()
	if err != nil {
		log.Fatalf("Failed to compile graph: %v", err)
	}

	// Prepare input messages
	system := message.NewSystemMessageFromText("You are a helpful assistant that provides concise answers.")
	human := message.NewHumanMessageFromText("Explain what graph streaming is in 2 sentences.")
	messages := []message.Message{system, human}

	ctx := context.Background()

	fmt.Println("🚀 Starting streaming execution...")
	fmt.Println("📊 Watch as nodes emit intermediate results using StreamWriter")
	fmt.Println(strings.Repeat("=", 70))

	// Stream the graph execution
	seq := compiled.Run(ctx, messages)

	// Track execution progress
	eventCount := 0
	currentNode := ""

	for event, err := range seq {
		if err != nil {
			log.Fatalf("Streaming error: %v", err)
		}

		if event.Node != currentNode {
			if currentNode != "" {
				fmt.Println() // Newline after previous node's output
			}
			currentNode = event.Node
			fmt.Printf("\n▶️ Executing Node: %s\n", aurora.Bold(aurora.Cyan(currentNode)))
			fmt.Println(strings.Repeat("-", 30))
		}

		if event.Message != nil && event.Message.Type() == message.TypeAI {
			// Print partial AI responses as they stream in
			if aiMsg, ok := event.Message.(*message.AIMessage); ok {
				for _, part := range aiMsg.Parts() {
					if textPart, ok := part.(message.TextPart); ok {
						fmt.Print(aurora.Green(textPart.Text))
					}
				}
			}
		}
		eventCount++
	}

	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Printf("\n✅ Streaming completed! Received %d total events\n", eventCount)

	// Display final state
	finalState := compiled.State()
	if finalState != nil {
		fmt.Println("\n� Final State:")
		for key, val := range finalState.GetAll() {
			fmt.Printf("   %s = %v\n", key, val)
		}

		// Show final messages
		finalEvents := finalState.MessagesSnapshot()
		if len(finalEvents) > 0 {
			fmt.Println("\n💬 Final Messages:")
			for i, evt := range finalEvents {
				content := ""
				for _, part := range evt.Message.Parts() {
					if textPart, ok := part.(message.TextPart); ok {
						content += textPart.Text
					}
				}
				if content != "" {
					fmt.Printf("   [%d] %s: %s\n", i+1, evt.Message.Type(), content)
				}
			}
		}
	}
}
