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
	lcg "github.com/hupe1980/agentmesh/model/langchaingo"
	lcgtools "github.com/hupe1980/agentmesh/tool/langchaingo"
	lcgopenai "github.com/tmc/langchaingo/llms/openai"
	lctools "github.com/tmc/langchaingo/tools"
)

func main() {
	if os.Getenv("OPENAI_API_KEY") == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required for the underlying OpenAI provider")
	}

	// Create the langchaingo OpenAI LLM (reads OPENAI_API_KEY from env), then wrap with our adapter.
	llm, err := lcgopenai.New(
		lcgopenai.WithModel("gpt-4o-mini"), // pick a lightweight default; change as desired
	)
	if err != nil {
		log.Fatalf("failed creating langchaingo OpenAI LLM: %v", err)
	}

	model, err := lcg.NewModel(llm)
	if err != nil {
		log.Fatalf("failed creating adapter: %v", err)
	}

	ag, err := am.NewModelAgent("LangChainGoAgent", model, func(o *am.ModelAgentOptions) {
		o.Instructions = am.NewInstructionsFromText(
			"You are a concise assistant. Use the calc tool when arithmetic is involved.",
		)
		// Wrap the langchaingo Calculator tool using our adapter
		o.Tools = []core.Tool{lcgtools.NewTool(&lctools.Calculator{})}
	})
	if err != nil {
		log.Fatalf("failed creating agent: %v", err)
	}

	application := am.NewApp("langchaingo_example", ag)

	r := am.NewRunner(application, func(o *am.RunnerOptions) {
		o.Logger = logging.NewSlogLogger(logging.LogLevelInfo, logging.LogFormatText, false)
	})
	defer func() { _ = r.Close() }()

	userParts := []am.Part{am.NewPartFromText("What is (12.5 + 7.5) * 2?")}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	runID, results, err := r.Run(ctx, "user1", "sess1", userParts)
	if err != nil {
		log.Fatalf("invoke failed: %v", err)
	}

	fmt.Printf("=== LangChainGo (OpenAI) Example [runID=%s] ===\n", runID)
	for res := range results {
		if res.Err != nil {
			log.Printf("error: %v", res.Err)
			continue
		}
		if res.Event == nil {
			continue
		}
		for _, p := range res.Event.Parts {
			switch v := p.(type) {
			case *core.TextPart:
				fmt.Printf("\n[%s]\n%s\n", res.Event.Author, v.Text)
			case *core.FunctionCallPart:
				fmt.Printf("\n[%s -> function_call]\n%s %s\n", res.Event.Author, v.FunctionCall.Name, v.FunctionCall.Arguments)
			case *core.FunctionResponsePart:
				fmt.Printf("\n[%s -> function_response]\n%s => %v\n", res.Event.Author, v.FunctionResponse.Name, v.FunctionResponse.Response)
			}
		}
	}
}
