// Package main demonstrates real-time streaming execution in AgentMesh.
//
// This example shows how to:
//   - Use the iterator-based execution model for streaming results
//   - Track execution progress with real-time result handling
//   - Stream LLM responses for better UX
//
// Prerequisites:
//
//	export OPENAI_API_KEY="sk-..."
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
	"github.com/hupe1980/agentmesh/pkg/event"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	pkgmodel "github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/model/openai"
	"github.com/logrusorgru/aurora/v3"
)

// Define state keys
var (
	progressKey     = graph.NewKey("progress", "")
	currentChunkKey = graph.NewKey("current_chunk", "")
	statusKey       = graph.NewKey("status", "")
	chunksTotalKey  = graph.NewKey("chunks_total", 0)
	stepKey         = graph.NewKey("step", "")
	qualityScoreKey = graph.NewKey("quality_score", 0.0)
	verifiedKey     = graph.NewKey("verified", false)
)

func main() {
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		log.Fatalf("OPENAI_API_KEY not set")
	}

	model := openai.NewModel()

	// Create a message graph using message.Message types for LLM integration
	g := graph.New[[]message.Message, message.Message](
		agent.MessagesKey, // List key for messages - this is the output key
		progressKey,
		currentChunkKey,
		statusKey,
		chunksTotalKey,
		stepKey,
		qualityScoreKey,
		verifiedKey,
	)

	// Node 1: Data processor with intermediate streaming
	g.Node("data_processor", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		streamWriter := graph.GetStreamWriter(ctx)

		// Simulate processing multiple chunks and streaming progress
		chunks := []string{"chunk1", "chunk2", "chunk3", "chunk4"}
		for i, chunk := range chunks {
			time.Sleep(300 * time.Millisecond) // Simulate processing time

			// Emit intermediate progress via stream
			if streamWriter != nil {
				streamWriter(graph.Updates{
					progressKey.Name():     fmt.Sprintf("%d/%d", i+1, len(chunks)),
					currentChunkKey.Name(): chunk,
				})
			}
		}

		return graph.Set(statusKey, "data_processed").
			With(graph.SetValue(chunksTotalKey, len(chunks))).
			To("llm_call")
	}, "llm_call")

	// Node 2: LLM call
	g.Node("llm_call", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		// Get messages from state using ListKey
		msgs := graph.GetList(view, agent.MessagesKey)

		// Create request and call model
		req := &pkgmodel.Request{Messages: msgs}
		resp, err := pkgmodel.Last(model.Generate(ctx, req))
		if err != nil {
			return graph.Fail(err)
		}

		return graph.Set(statusKey, "llm_completed").
			With(graph.AppendValue(agent.MessagesKey, resp.Message)).
			To("analyzer")
	}, "analyzer")

	// Node 3: Multi-step analyzer with streaming progress
	g.Node("analyzer", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		streamWriter := graph.GetStreamWriter(ctx)

		steps := []struct {
			name   string
			result string
		}{
			{"validation", "passed"},
			{"quality_check", "score 0.95"},
			{"finalization", "ready"},
		}

		for _, step := range steps {
			time.Sleep(300 * time.Millisecond)

			// Stream each step's progress
			if streamWriter != nil {
				streamWriter(graph.Updates{
					stepKey.Name(): fmt.Sprintf("%s: %s", step.name, step.result),
				})
			}
		}

		return graph.Set(statusKey, "analysis_complete").
			With(graph.SetValue(verifiedKey, true)).
			With(graph.SetValue(qualityScoreKey, 0.95)).
			End()
	}, graph.END)

	g.Start("data_processor")

	compiled, err := g.Build()
	if err != nil {
		log.Fatalf("Failed to build graph: %v", err)
	}

	// Prepare input messages
	system := message.NewSystemMessageFromText("You are a helpful assistant that provides concise answers.")
	human := message.NewHumanMessageFromText("Explain what graph streaming is in 2 sentences.")
	messages := []message.Message{system, human}

	// Create event bus to receive streaming updates
	bus := event.NewBus()
	bus.Subscribe(event.HandlerFunc(func(ctx context.Context, e event.Event) error {
		if e.Type == event.EventStateUpdate {
			if updates, ok := e.Data["updates"].(graph.Updates); ok {
				// Print progress updates
				if progress, ok := updates[progressKey.Name()]; ok {
					fmt.Printf("   📊 Progress: %v", progress)
					if chunk, ok := updates[currentChunkKey.Name()]; ok {
						fmt.Printf(" (processing %v)", chunk)
					}
					fmt.Println()
				}
				// Print step updates
				if step, ok := updates[stepKey.Name()]; ok {
					fmt.Printf("   🔍 Step: %v\n", step)
				}
			}
		} else if e.Type == event.EventNodeStart {
			fmt.Printf("\n   ⏳ Starting node: %s\n", e.Node)
		} else if e.Type == event.EventNodeComplete {
			fmt.Printf("   ✓ Completed node: %s (took %v)\n", e.Node, e.Duration)
		}
		return nil
	}), event.EventStateUpdate, event.EventNodeStart, event.EventNodeComplete)

	ctx := event.WithBus(context.Background(), bus)

	fmt.Println("🚀 Starting streaming execution...")
	fmt.Println("📊 Watch as nodes execute and emit results")
	fmt.Println(strings.Repeat("=", 70))

	// Stream the graph execution using iterator
	eventCount := 0
	for msg, err := range compiled.Run(ctx, messages) {
		if err != nil {
			log.Fatalf("Execution error: %v", err)
		}

		if msg.Type() == message.TypeAI {
			// Print AI responses as they arrive
			if aiMsg, ok := msg.(*message.AIMessage); ok {
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
}
