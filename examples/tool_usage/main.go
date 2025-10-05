package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"time"

	am "github.com/hupe1980/agentmesh"
	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/logging"
	"github.com/hupe1980/agentmesh/model/openai"
	"github.com/hupe1980/agentmesh/tool"
)

// Build a calculator tool using the generic FuncTool adapter.
func newCalculatorTool() core.Tool {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"operation": map[string]any{
				"type":        "string",
				"description": "Operation",
				"enum":        []string{"add", "subtract", "multiply", "divide", "power", "sqrt"},
			},
			"a": map[string]any{
				"type":        "number",
				"description": "First number",
			},
			"b": map[string]any{
				"type":        "number",
				"description": "Second number (required for add, subtract, multiply, divide, power; not used for sqrt)",
			},
		},
		"required": []string{"operation", "a"},
	}

	fn := func(_ context.Context, _ core.ToolContext, args map[string]any) (any, error) {
		op, _ := args["operation"].(string)
		aVal, aOK := args["a"]
		a, aCast := aVal.(float64)
		if !aOK || !aCast {
			return 0, fmt.Errorf("missing or invalid parameter 'a'")
		}

		var b float64
		if op == "add" || op == "subtract" || op == "multiply" || op == "divide" || op == "power" {
			bVal, bOK := args["b"]
			bCast, ok := bVal.(float64)
			if !bOK || !ok {
				return 0, fmt.Errorf("missing or invalid parameter 'b' for operation '%s'", op)
			}
			b = bCast
		}

		switch op {
		case "add":
			return a + b, nil
		case "subtract":
			return a - b, nil
		case "multiply":
			return a * b, nil
		case "divide":
			if b == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			return a / b, nil
		case "power":
			return math.Pow(a, b), nil
		case "sqrt":
			if a < 0 {
				return 0, fmt.Errorf("sqrt negative")
			}
			return math.Sqrt(a), nil
		default:
			return 0, fmt.Errorf("unsupported operation: %s", op)
		}
	}

	return tool.NewFuncTool("calculator", "Perform basic math operations (add, subtract, multiply, divide, power, sqrt)", schema, fn)
}

func main() {
	if os.Getenv("OPENAI_API_KEY") == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}

	model := openai.NewModel(func(o *openai.Options) {
		o.Temperature = 0
	})

	calcAgent, err := am.NewModelAgent("CalculatorAgent", model, func(o *am.ModelAgentOptions) {
		o.Instructions = am.NewInstructionsFromText(
			"You are a careful math assistant.\n" +
				"- Use the calculator tool for each numeric step.\n" +
				"- For squaring a value x, call operation 'power' with a=x and b=2 (always include b).\n" +
				"- Circle area = 3.14159 * r^2 (compute r^2 first, then multiply by 3.14159).\n" +
				"- Square area (side s) = s^2 using 'power' with a=s, b=2.\n" +
				"- Percent = (circle_area / square_area) * 100.\n" +
				"- Always provide both 'a' and 'b' for binary operations (add, subtract, multiply, divide, power).\n" +
				"Then summarize the final numeric answer.",
		)
		o.Tools = []core.Tool{newCalculatorTool()}
	})
	if err != nil {
		log.Fatalf("failed creating agent: %v", err)
	}

	application := am.NewApp("calculator_app", calcAgent)

	r := am.NewRunner(application, func(o *am.RunnerOptions) {
		o.Logger = logging.NewSlogLogger(logging.LogLevelInfo, logging.LogFormatText, false)
	})
	defer func() {
		_ = r.Close()
	}()

	userParts := []am.Part{am.NewPartFromText(
		"Calculate the area of a circle with radius 5.5, " +
			"then what percent of a square of side 12 it is.",
	)}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	runID, results, err := r.Run(ctx, "user1", "sess1", userParts)
	if err != nil {
		log.Fatalf("invoke failed: %v", err)
	}

	fmt.Printf("=== Tool Usage (Calculator) [runID=%s] ===\n", runID)
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
