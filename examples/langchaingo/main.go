package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"time"

	"github.com/hupe1980/agentmesh/agent"
	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/logging"
	lcg "github.com/hupe1980/agentmesh/model/langchaingo"
	"github.com/hupe1980/agentmesh/runner"
	"github.com/hupe1980/agentmesh/tool"
	lcgopenai "github.com/tmc/langchaingo/llms/openai"
)

// newCalculatorTool returns a simple calculator tool to demonstrate tool-use via the langchaingo adapter.
func newCalculatorTool() core.Tool {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"op": map[string]any{
				"type":        "string",
				"description": "Operation to perform",
				"enum":        []string{"add", "sub", "mul", "div", "pow"},
			},
			"a": map[string]any{
				"type":        "number",
				"description": "First operand",
			},
			"b": map[string]any{
				"type":        "number",
				"description": "Second operand (required except maybe pow depending on usage)",
			},
		},
		"required": []string{"op", "a"},
	}

	fn := func(_ context.Context, _ core.ToolContext, args map[string]any) (any, error) {
		op, _ := args["op"].(string)
		av, ok := args["a"].(float64)
		if !ok {
			return nil, fmt.Errorf("invalid or missing 'a'")
		}
		var bv float64
		if braw, has := args["b"]; has {
			if v, ok := braw.(float64); ok {
				bv = v
			}
		}
		switch op {
		case "add":
			return av + bv, nil
		case "sub":
			return av - bv, nil
		case "mul":
			return av * bv, nil
		case "div":
			if bv == 0 {
				return nil, fmt.Errorf("division by zero")
			}
			return av / bv, nil
		case "pow":
			return math.Pow(av, bv), nil
		default:
			return nil, fmt.Errorf("unsupported op: %s", op)
		}
	}

	return tool.NewFuncTool("calc", "Simple calculator supporting add/sub/mul/div/pow", schema, fn)
}

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

	ag := agent.NewModelAgent("LangChainGoAgent", model, func(o *agent.ModelAgentOptions) {
		o.Instructions = agent.NewInstructionsFromText(
			"You are a concise assistant. Use the calc tool when arithmetic is involved.",
		)
		o.Tools = []core.Tool{newCalculatorTool()}
	})

	r := runner.New("langchaingo_example", ag, func(o *runner.Options) {
		o.Logger = logging.NewSlogLogger(logging.LogLevelInfo, logging.LogFormatText, false)
	})
	defer func() { _ = r.Close() }()

	userParts := []core.Part{core.NewPartFromText("What is (12.5 + 7.5) * 2? Show steps briefly.")}

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
