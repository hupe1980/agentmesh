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

	"github.com/hupe1980/agentmesh/pkg/exec"
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
	builder, err := exec.NewBuilder()
	if err != nil {
		log.Fatalf("Failed to create builder: %v", err)
	}

	progressKey := graphstate.NewKey("progress", "")
	currentChunkKey := graphstate.NewKey("current_chunk", "")
	statusKey := graphstate.NewKey("status", "")
	chunksTotalKey := graphstate.NewKey("chunks_total", 0)

	// Node 1: Data processor with intermediate streaming
	builder.Node("data_processor", func(ctx context.Context, view *graphstate.ReadView) (*graph.NodeResult, error) {
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
					Updates: graphstate.Updates{
						progressKey.Name():     fmt.Sprintf("%d/%d", i+1, len(chunks)),
						currentChunkKey.Name(): chunk,
					},
				})
			}
		}

		return &graph.NodeResult{
			Updates: graphstate.Updates{
				statusKey.Name():      "data_processed",
				chunksTotalKey.Name(): len(chunks),
			},
		}, nil
	})

	// Node 2: LLM call with streaming
	builder.Node("llm_call", func(ctx context.Context, view *graphstate.ReadView) (*graph.NodeResult, error) {
		streamWriter := graph.GetStreamWriter(ctx)

		fmt.Println("   ⏳ Calling LLM...")

		llmStatusKey := graphstate.NewKey("llm_status", "")

		// Emit pre-call status
		if streamWriter != nil {
			streamWriter(&graph.NodeResult{
				Updates: graphstate.Updates{
					llmStatusKey.Name(): "starting",
				},
			})
		}

		// Get messages from state
		events := graphstate.GetMessages(view)
		msgs := graphstate.ExtractMessageContent(events)

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
				Updates: graphstate.Updates{
					llmStatusKey.Name(): "completed",
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
	analysisStepKey := graphstate.NewKey("analysis_step", "")
	validationKey := graphstate.NewKey("validation", "")
	qualityScoreKey := graphstate.NewKey("quality_score", 0.0)

	builder.Node("analyzer", func(ctx context.Context, view *graphstate.ReadView) (*graph.NodeResult, error) {
		streamWriter := graph.GetStreamWriter(ctx)

		fmt.Println("   ⏳ Analyzing results...")

		// Step 1: Validation
		time.Sleep(300 * time.Millisecond)
		if streamWriter != nil {
			streamWriter(&graph.NodeResult{
				Updates: graphstate.Updates{
					analysisStepKey.Name(): "validation",
					validationKey.Name():   "passed",
				},
			})
		}

		// Step 2: Quality check
		time.Sleep(300 * time.Millisecond)
		if streamWriter != nil {
			streamWriter(&graph.NodeResult{
				Updates: graphstate.Updates{
					analysisStepKey.Name(): "quality_check",
					qualityScoreKey.Name(): 0.95,
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
	st := builder.State()
	if st != nil {
		snap := st.Snapshot()
		finalView := graphstate.NewReadView(snap)

		fmt.Println("\n📊 Final State:")
		fmt.Printf("   status = %v\n", graphstate.GetFromView(finalView, statusKey))
		fmt.Printf("   chunks_total = %v\n", graphstate.GetFromView(finalView, chunksTotalKey))

		// Show final messages
		finalEvents := graphstate.GetMessages(finalView)
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
