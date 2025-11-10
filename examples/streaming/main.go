// Package main demonstrates real-time streaming execution in AgentMesh.
//
// This example shows how to:
//   - Use Stream for live updates during graph execution
//   - Stream partial results as nodes complete
//   - Track execution progress with real-time event handling
//   - Stream LLM responses token-by-token for better UX
//   - Handle cleanup with defer stream.Close() to prevent goroutine leaks
//
// Key concepts:
//   - Stream: Real-time event channel for graph execution
//   - Event Types: NodeStart, NodeComplete, NodeError, GraphComplete
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

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	pkgmodel "github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/model/openai"
)

func main() {
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		log.Fatalf("OPENAI_API_KEY not set")
	}

	model := openai.NewModel()

	// Build a multi-node graph to demonstrate streaming
	builder := graph.NewBuilder()

	// Node 1: Data processor with intermediate streaming
	builder.Node("data_processor", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
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
	builder.Node("llm_call", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
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
		events := s.MessageEventsSnapshot()
		msgs := graph.ExtractMessages(events)

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
	builder.Node("analyzer", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
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
	stream, err := compiled.Stream(ctx, messages)
	if err != nil {
		log.Fatalf("Failed to start stream: %v", err)
	}
	defer stream.Cancel()

	// Track execution progress
	eventCount := 0
	currentNode := ""

	// Process streaming events
	for stream.Next() {
		event := stream.Current()
		eventCount++

		// Handle errors
		if event.Err != nil {
			fmt.Printf("\n❌ Error in node %q: %v\n", event.Node, event.Err)
			continue
		}

		// Display node execution events
		if event.Node != "" {
			// New node starting
			if event.Node != currentNode && event.Result == nil {
				currentNode = event.Node
				fmt.Printf("\n📍 Node: %s\n", event.Node)
			}

			// Show intermediate updates (from StreamWriter calls)
			if event.Result != nil && len(event.Result.Updates) > 0 {
				fmt.Printf("   ⚡ Intermediate: ")
				for key, val := range event.Result.Updates {
					fmt.Printf("%s=%v ", key, val)
				}
				fmt.Println()
			}

			// Show state updates (final result)
			if len(event.Updates) > 0 && event.Result == nil {
				fmt.Printf("   ✅ Final: ")
				for key, val := range event.Updates {
					fmt.Printf("%s=%v ", key, val)
				}
				fmt.Println()
			}

			// Show messages
			if len(event.Messages) > 0 {
				for _, msg := range event.Messages {
					msgType := "Unknown"
					switch msg.Type() {
					case message.TypeHuman:
						msgType = "👤 Human"
					case message.TypeAI:
						msgType = "🤖 AI"
					case message.TypeSystem:
						msgType = "⚙️  System"
					case message.TypeTool:
						msgType = "🔧 Tool"
					}

					// Extract content from message parts
					content := ""
					for _, part := range msg.Parts() {
						if textPart, ok := part.(message.TextPart); ok {
							content += textPart.Text
						}
					}

					if content != "" {
						// Truncate for display
						displayContent := content
						if len(displayContent) > 80 {
							displayContent = displayContent[:77] + "..."
						}
						fmt.Printf("   💬 %s: %s\n", msgType, displayContent)
					}
				}
			}
		}
	}

	// Check for stream errors
	if err := stream.Err(); err != nil {
		log.Fatalf("Stream error: %v", err)
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
		finalEvents := finalState.MessageEventsSnapshot()
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
