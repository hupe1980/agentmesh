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

// transfer_agent demonstrates dynamic delegation using the transfer_to_agent tool.
// The root agent is allowed to transfer to specialized child agents based on
// the user's request. The LLM will be shown the transfer_to_agent tool schema
// (in multi-agent flow) and can choose to call it with a target agent name.
func main() {
	if os.Getenv("OPENAI_API_KEY") == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}

	model := openai.NewModel()

	// Root agent: orchestrator that can decide to transfer.
	root := agent.NewModelAgent("RouterAgent", model, func(o *agent.ModelAgentOptions) {
		o.Instruction = agent.NewInstructionFromText(`You route the request to the best specialist. If the query is about math, transfer to MathAgent. If about history, transfer to HistoryAgent. Otherwise answer directly. When transferring, call transfer_to_agent with {"agent_name": "TargetName"}. After a transfer, the sub-agent answers.`)
	})

	// Specialist child agents
	mathAgent := agent.NewModelAgent("MathAgent", model, func(o *agent.ModelAgentOptions) {
		o.Instruction = agent.NewInstructionFromText("You are a math expert. Provide clear step-by-step solutions and final answers.")
	})

	historyAgent := agent.NewModelAgent("HistoryAgent", model, func(o *agent.ModelAgentOptions) {
		o.Instruction = agent.NewInstructionFromText("You are a history expert. Provide concise, factual historical answers.")
	})

	// Build hierarchy: root -> (math, history)
	_ = root.SetSubAgents(mathAgent, historyAgent)

	run := runner.New(root, func(o *runner.Options) {
		o.Logger = logging.NewSlogLogger(logging.LogLevelInfo, "text", false)
	})

	userContent := newUserText("What is the derivative of x^2 + 3x + 5?")

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	_, eventsCh, errorsCh, err := run.Run(ctx, "sess-transfer-1", userContent)
	if err != nil {
		log.Fatalf("invoke failed: %v", err)
	}

	fmt.Println("=== Transfer Agent Demo ===")
	consume(eventsCh, errorsCh)
}

func consume(eventsCh <-chan core.Event, errorsCh <-chan error) {
	for eventsCh != nil || errorsCh != nil {
		select {
		case ev, ok := <-eventsCh:
			if !ok {
				eventsCh = nil
				continue
			}
			if ev.Content != nil {
				printEvent(ev)
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

func printEvent(ev core.Event) {
	for _, p := range ev.Content.Parts {
		if tp, ok := p.(core.TextPart); ok {
			fmt.Printf("\n[%s]\n%s\n", ev.Author, tp.Text)
		}
		// Show function call/response parts for transparency
		if fc, ok := p.(core.FunctionCallPart); ok {
			fmt.Printf("\n[%s -> function_call]\n%s %s\n", ev.Author, fc.FunctionCall.Name, fc.FunctionCall.Arguments)
		}
		if fr, ok := p.(core.FunctionResponsePart); ok {
			fmt.Printf("\n[%s -> function_response]\n%s => %v\n", ev.Author, fr.FunctionResponse.Name, fr.FunctionResponse.Response)
		}
	}
}

func newUserText(txt string) core.Content {
	return core.Content{Role: "user", Parts: []core.Part{core.TextPart{Text: txt}}}
}
