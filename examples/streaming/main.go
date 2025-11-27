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

	"github.com/hupe1980/agentmesh/pkg/agent"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/command"
	"github.com/hupe1980/agentmesh/pkg/message"
	pkgmodel "github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/model/openai"
	graphstate "github.com/hupe1980/agentmesh/pkg/state"
	"github.com/logrusorgru/aurora/v3"
)

func main() {
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		log.Fatalf("OPENAI_API_KEY not set")
	}

	model := openai.NewModel()

	// Build a multi-node graph to demonstrate streaming
	builder, err := graph.NewBuilder(graph.NewMessagePregelExecutor())
	if err != nil {
		log.Fatalf("Failed to create builder: %v", err)
	}

	// Define state keys
	progressKey := graphstate.NewKey("progress", "")
	currentChunkKey := graphstate.NewKey("current_chunk", "")
	statusKey := graphstate.NewKey("status", "")
	chunksTotalKey := graphstate.NewKey("chunks_total", 0)
	llmStatusKey := graphstate.NewKey("llm_status", "")
	analysisStepKey := graphstate.NewKey("analysis_step", "")
	validationKey := graphstate.NewKey("validation", "")
	qualityScoreKey := graphstate.NewKey("quality_score", 0.0)
	readyKey := graphstate.NewKey("ready", false)
	verifiedKey := graphstate.NewKey("verified", false)

	// Register state keys before use
	mgr := builder.Manager()
	// Register messages key for MessagePregelExecutor (required for streaming)
	_ = graphstate.RegisterListKey(mgr, agent.MessagesKey)
	_ = graphstate.RegisterKey(mgr, progressKey)
	_ = graphstate.RegisterKey(mgr, currentChunkKey)
	_ = graphstate.RegisterKey(mgr, statusKey)
	_ = graphstate.RegisterKey(mgr, chunksTotalKey)
	_ = graphstate.RegisterKey(mgr, llmStatusKey)
	_ = graphstate.RegisterKey(mgr, analysisStepKey)
	_ = graphstate.RegisterKey(mgr, validationKey)
	_ = graphstate.RegisterKey(mgr, qualityScoreKey)
	_ = graphstate.RegisterKey(mgr, readyKey)
	_ = graphstate.RegisterKey(mgr, verifiedKey)

	builder.SetEntryPoint("data_processor")

	// Node 1: Data processor with intermediate streaming
	builder.AddNodeFunc("data_processor", []string{"llm_call"}, func(ctx context.Context, view graphstate.ReadView) ([]string, graphstate.Updates, error) {
		// Get the stream writer to emit intermediate results
		streamWriter := graph.GetStreamWriter(ctx)

		fmt.Println("   ⏳ Processing data in chunks...")

		// Simulate processing multiple chunks and streaming progress
		chunks := []string{"chunk1", "chunk2", "chunk3", "chunk4"}
		for i, chunk := range chunks {
			time.Sleep(300 * time.Millisecond) // Simulate processing time

			// Emit intermediate progress via stream
			if streamWriter != nil {
				updates, _ := command.New().
					Set(progressKey, fmt.Sprintf("%d/%d", i+1, len(chunks))).
					Set(currentChunkKey, chunk).
					Build()
				streamWriter(updates)
			}
		}

		updates, err := command.New().
			Set(statusKey, "data_processed").
			Set(chunksTotalKey, len(chunks)).
			Build()
		return []string{"llm_call"}, updates, err
	})

	// Node 2: LLM call with streaming
	builder.AddNodeFunc("llm_call", []string{"analyzer"}, func(ctx context.Context, view graphstate.ReadView) ([]string, graphstate.Updates, error) {
		streamWriter := graph.GetStreamWriter(ctx)

		fmt.Println("   ⏳ Calling LLM...")

		// Emit pre-call status
		if streamWriter != nil {
			updates, _ := command.New().Set(llmStatusKey, "starting").Build()
			streamWriter(updates)
		}

		// Get messages from state
		msgs := agent.GetMessages(view)

		// Create request
		req := &pkgmodel.Request{
			Messages: msgs,
		} // Call the model
		resp, err := pkgmodel.Last(model.Generate(ctx, req))
		if err != nil {
			return nil, nil, err
		}

		// Emit post-call status
		if streamWriter != nil {
			streamUpdates, _ := command.New().Set(llmStatusKey, "completed").Build()
			streamWriter(streamUpdates)
		}

		updates, err := command.New().
			Set(statusKey, "llm_completed").
			Set(agent.MessagesKey, []message.Message{resp.Message}).
			Build()
		return []string{"analyzer"}, updates, err
	})

	// Node 3: Multi-step analyzer with detailed streaming
	builder.AddNodeFunc("analyzer", []string{graph.EndNode}, func(ctx context.Context, view graphstate.ReadView) ([]string, graphstate.Updates, error) {
		streamWriter := graph.GetStreamWriter(ctx)

		fmt.Println("   ⏳ Analyzing results...")

		// Step 1: Validation
		time.Sleep(300 * time.Millisecond)
		if streamWriter != nil {
			updates, _ := command.New().
				Set(analysisStepKey, "validation").
				Set(validationKey, "passed").
				Build()
			streamWriter(updates)
		}

		// Step 2: Quality check
		time.Sleep(300 * time.Millisecond)
		if streamWriter != nil {
			updates, _ := command.New().
				Set(analysisStepKey, "quality_check").
				Set(qualityScoreKey, 0.95).
				Build()
			streamWriter(updates)
		}

		// Step 3: Finalization
		time.Sleep(300 * time.Millisecond)
		if streamWriter != nil {
			updates, _ := command.New().
				Set(analysisStepKey, "finalization").
				Set(readyKey, true).
				Build()
			streamWriter(updates)
		}

		updates, err := command.New().
			Set(statusKey, "analysis_complete").
			Set(verifiedKey, true).
			Build()
		return []string{graph.EndNode}, updates, err
	})

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

	for event, err := range seq {
		if err != nil {
			log.Fatalf("Streaming error: %v", err)
		}

		if event != nil && event.Type() == message.TypeAI {
			// Print partial AI responses as they stream in
			if aiMsg, ok := event.(*message.AIMessage); ok {
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
	if mgr != nil {
		finalView, err := mgr.CreateReadView(ctx)
		if err != nil {
			fmt.Printf("Error creating read view: %v\n", err)
			return
		}

		fmt.Println("\n📊 Final State:")
		fmt.Printf("   status = %v\n", graphstate.GetFromView(finalView, statusKey))
		fmt.Printf("   chunks_total = %v\n", graphstate.GetFromView(finalView, chunksTotalKey))

		// Show final messages
		finalMessages := agent.GetMessages(finalView)
		if len(finalMessages) > 0 {
			fmt.Println("\n💬 Final Messages:")
			for i, msg := range finalMessages {
				content := ""
				for _, part := range msg.Parts() {
					if textPart, ok := part.(message.TextPart); ok {
						content += textPart.Text
					}
				}
				if content != "" {
					fmt.Printf("   [%d] %s: %s\n", i+1, msg.Type(), content)
				}
			}
		}
	}
}
