package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/hupe1980/agentmesh"
	"github.com/hupe1980/agentmesh/agent"
	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/logging"
	"github.com/hupe1980/agentmesh/model/openai"
)

// basic_agent demonstrates the smallest useful LLM agent: a single model-backed agent
// that responds to a user message. It shows the canonical initialization pattern
// used across all examples (logger, model, agent registration, invocation loop).
func main() {
	if os.Getenv("OPENAI_API_KEY") == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}

	// 1. Create the mesh with a standard logger
	mesh := agentmesh.New(func(o *agentmesh.Options) {
		o.Logger = logging.NewSlogLogger(logging.LogLevelInfo, "text", false)
	})

	// 2. Create model + agent with an instruction prompt
	model := openai.NewModel()
	llmAgent := agent.NewModelAgent("BasicAgent", model, func(o *agent.ModelAgentOptions) {
		o.Instruction = agent.NewInstructionFromText("You are a helpful assistant. Keep responses concise and friendly.")
	})
	mesh.RegisterAgent(llmAgent)

	// 3. Build user content (helper function style across examples)
	userContent := newUserText("Hello! What can you help me with?")

	// 4. Invoke agent with timeout context
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	_, eventsCh, errorsCh, err := mesh.Invoke(ctx, "session-basic", llmAgent.Name(), userContent)
	if err != nil {
		log.Fatalf("invoke failed: %v", err)
	}

	fmt.Println("=== Basic Agent ===")
	consumeEvents(eventsCh, errorsCh, llmAgent.Name())
}

// consumeEvents is a reusable event loop pattern used in all examples for consistency.
func consumeEvents(eventsCh <-chan core.Event, errorsCh <-chan error, focusAgent string) {
	for eventsCh != nil || errorsCh != nil {
		select {
		case ev, ok := <-eventsCh:
			if !ok {
				eventsCh = nil
				continue
			}
			if ev.Content != nil && ev.Author == focusAgent {
				printTextParts(ev.Content.Parts)
			}
		case err, ok := <-errorsCh:
			if !ok {
				errorsCh = nil
				continue
			}
			if err != nil {
				log.Printf("error: %v", err)
			}
		}
	}
}

func printTextParts(parts []core.Part) {
	for _, p := range parts {
		if tp, ok := p.(core.TextPart); ok {
			fmt.Printf("%s\n", tp.Text)
		}
	}
}

func newUserText(txt string) core.Content {
	return core.Content{Role: "user", Parts: []core.Part{core.TextPart{Text: txt}}}
}
