package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/hupe1980/agentmesh/agent"
	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/logging"
	"github.com/hupe1980/agentmesh/model/openai"
	"github.com/hupe1980/agentmesh/runner"
)

// basic_agent demonstrates the smallest useful LLM agent: a single model-backed agent
// that responds to a user message. It shows the canonical initialization pattern
// used across all examples (logger, model, agent registration, invocation loop).
func main() {
	if os.Getenv("OPENAI_API_KEY") == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}

	// 1. Create model + agent with an instruction prompt
	model := openai.NewModel()

	llmAgent := agent.NewModelAgent("BasicAgent", model, func(o *agent.ModelAgentOptions) {
		o.Instructions = agent.NewInstructionsFromText(
			"You are a helpful assistant. Keep responses concise and friendly.",
		)
	})

	// 2. Create the runner with a standard logger
	r := runner.New("basic_agent_app", llmAgent, func(o *runner.Options) {
		o.Logger = logging.NewSlogLogger(logging.LogLevelInfo, logging.LogFormatText, false)
	})
	defer func() {
		_ = r.Close()
	}()

	// 3. Build user content (helper function style across examples)
	userParts := []core.Part{core.NewPartFromText("Hello! What can you help me with?")}

	// 4. Invoke agent with timeout context
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	runID, results, err := r.Run(ctx, "user1", "sess1", userParts)
	if err != nil {
		log.Fatalf("invoke failed: %v", err)
	}

	fmt.Printf("=== Basic Agent [runID=%s] ===\n", runID)
	consume(results, llmAgent.Name())
}

// consume handles RunResult stream (events + errors unified).
func consume(results <-chan core.RunResult, focus string) {
	for res := range results {
		if res.Err != nil {
			log.Printf("error: %v", res.Err)
			continue
		}

		if res.Event.Author == focus {
			for _, p := range res.Event.Parts {
				if tp, ok := p.(*core.TextPart); ok {
					fmt.Println(tp.Text)
				}
			}
		}
	}
}
