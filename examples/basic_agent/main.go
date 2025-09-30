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

	llmAgent, err := agent.NewModelAgent("BasicAgent", model, func(o *agent.ModelAgentOptions) {
		o.Instructions = agent.NewInstructionsFromText(
			"You are a helpful assistant. Keep responses concise and friendly.",
		)
	})
	if err != nil {
		log.Fatalf("failed to create agent: %v", err)
	}

	// 2. Create the runner with a standard logger
	r := runner.New("basic_agent_app", llmAgent, func(o *runner.Options) {
		o.Logger = logging.NewSlogLogger(logging.LogLevelInfo, logging.LogFormatText, false)
	})
	defer func() {
		_ = r.Close()
	}()

	// 3. Build user content (helper function style across examples)
	userParts := []core.Part{core.NewPartFromText("Hello! What can you help me with?")}

	// 4. Invoke agent with timeout context and get only the final text
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	runID, text, err := runner.RunFinalText(ctx, r, "user1", "sess1", userParts)
	if err != nil {
		log.Fatalf("invoke failed: %v", err)
	}

	fmt.Printf("=== Basic Agent [runID=%s] ===\n%s\n", runID, text)
}
