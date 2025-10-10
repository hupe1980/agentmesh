package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	am "github.com/hupe1980/agentmesh"
	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/logging"
	"github.com/hupe1980/agentmesh/model/openai"
)

// transfer_agent demonstrates dynamic delegation using the transfer_to_agent tool.
// The root agent is allowed to transfer to specialized child agents based on
// the user's request. The LLM will be shown the transfer_to_agent tool schema
// (in multi-agent flow) and can choose to call it with a target agent name.
func main() {
	if os.Getenv("OPENAI_API_KEY") == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}

	model := openai.NewModel(func(o *openai.Options) {
		o.Temperature = 0
	})

	// Specialist child agents
	mathAgent, err := am.NewModelAgent("MathAgent", model, func(o *am.ModelAgentOptions) {
		o.Instructions = core.NewInstructionsFromText(
			"You are a math expert.\n" +
				"- Solve with clear, concise steps and provide a boxed final answer.\n" +
				"- For calculus, apply the power rule/product rule/chain rule as appropriate.\n" +
				"- Keep the explanation short (3-6 lines) unless complexity requires more.",
		)
		o.Description = "Expert in mathematics: algebra, calculus (derivatives, integrals), and general calculations."
		o.AllowTransferToParent = false
	})
	if err != nil {
		log.Fatalf("failed creating math agent: %v", err)
	}

	historyAgent, err := am.NewModelAgent("HistoryAgent", model, func(o *am.ModelAgentOptions) {
		o.Instructions = core.NewInstructionsFromText(
			"You are a history expert.\n" +
				"- Provide concise, factual answers with dates and key names when available.\n" +
				"- Avoid speculation; indicate uncertainty if sources conflict.",
		)
		o.Description = "Expert in historical facts, events, timelines, and context."
		o.AllowTransferToParent = false
	})
	if err != nil {
		log.Fatalf("failed creating history agent: %v", err)
	}

	// Build hierarchy at construction: root -> (math, history)
	root, err := am.NewModelAgent("RouterAgent", model, func(o *am.ModelAgentOptions) {
		o.Instructions = core.NewInstructionsFromText(
			"You are a routing assistant. Not a subject-matter expert. Prefer delegating to specialists; answer directly only if no specialist fits.",
		)
		o.Description = "Routing orchestrator. Not a subject-matter expert. Prefer delegating to specialists; answer directly only if no specialist fits."
		o.SubAgents = []core.Agent{mathAgent, historyAgent}
	})
	if err != nil {
		log.Fatalf("failed creating root agent: %v", err)
	}

	// Application runner

	application := am.NewApp("transfer_app", root)

	r := am.NewRunner(application, func(o *am.RunnerOptions) {
		o.Logger = logging.NewSlogLogger(logging.LogLevelInfo, logging.LogFormatText, false)
	})
	defer func() {
		_ = r.Close()
	}()

	userParts := []core.Part{core.NewPartFromText("What is the derivative of x^2 + 3x + 5?")}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	runID, results, err := r.Run(ctx, "user1", "sess1", userParts)
	if err != nil {
		log.Fatalf("invoke failed: %v", err)
	}

	fmt.Printf("=== Transfer Agent Demo (runID=%s) ===\n", runID)
	consume(results)
}

func consume(results <-chan core.RunResult) {
	for res := range results {
		if res.Err != nil {
			log.Printf("error: %v", res.Err)
			continue
		}

		ev := res.Event
		if ev == nil || len(ev.Parts) == 0 {
			continue
		}

		printParts(ev)
	}
}

func printParts(ev *core.Event) {
	for _, p := range ev.Parts {
		switch v := p.(type) {
		case *core.TextPart:
			fmt.Printf(
				"\n[%s]\n%s\n",
				ev.Author,
				v.Text,
			)
		case *core.FunctionCallPart:
			fmt.Printf(
				"\n[%s -> function_call]\n%s %s\n",
				ev.Author,
				v.FunctionCall.Name,
				v.FunctionCall.Arguments,
			)
		case *core.FunctionResponsePart:
			fmt.Printf(
				"\n[%s -> function_response]\n%s => %v\n",
				ev.Author,
				v.FunctionResponse.Name,
				v.FunctionResponse.Response,
			)
		}
	}
}
