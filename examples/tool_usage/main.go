package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"time"

	"github.com/hupe1980/agentmesh"
	"github.com/hupe1980/agentmesh/agent"
	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/logging"
	"github.com/hupe1980/agentmesh/model/openai"
)

// CalculatorTool demonstrates a custom tool with parameter schema.
type CalculatorTool struct{}

func (t *CalculatorTool) Name() string {
	return "calculator"
}

func (t *CalculatorTool) Description() string {
	return "Perform basic math operations (add, subtract, multiply, divide, power, sqrt)"
}

func (t *CalculatorTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"operation": map[string]interface{}{
				"type":        "string",
				"description": "Operation",
				"enum":        []string{"add", "subtract", "multiply", "divide", "power", "sqrt"},
			},
			"a": map[string]interface{}{
				"type":        "number",
				"description": "First number",
			},
			"b": map[string]interface{}{
				"type":        "number",
				"description": "Second number (required for add, subtract, multiply, divide, power; not used for sqrt)",
			},
		},
		"required": []string{"operation", "a"},
	}
}

func (t *CalculatorTool) Call(toolCtx *core.ToolContext, args map[string]interface{}) (interface{}, error) {
	op, _ := args["operation"].(string)

	a, _ := args["a"].(float64)
	b, _ := args["b"].(float64)

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
	}
	return 0, fmt.Errorf("unsupported op")
}

func main() {
	if os.Getenv("OPENAI_API_KEY") == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}

	model := openai.NewModel()

	calcAgent := agent.NewModelAgent("CalculatorAgent", model, func(o *agent.ModelAgentOptions) {
		o.Instruction = agent.NewInstructionFromText("Use the calculator tool to perform intermediate steps, then summarize the final answer.")
	})
	calcAgent.RegisterTool(&CalculatorTool{})

	mesh := agentmesh.New(calcAgent, func(o *agentmesh.Options) {
		o.Logger = logging.NewSlogLogger(logging.LogLevelInfo, "text", false)
	})

	userContent := newUserText("Calculate the area of a circle with radius 5.5, then what percent of a square of side 12 it is.")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	_, eventsCh, errorsCh, err := mesh.Invoke(ctx, "sess1", userContent)
	if err != nil {
		log.Fatalf("invoke failed: %v", err)
	}

	fmt.Println("=== Tool Usage (Calculator) ===")
	consume(eventsCh, errorsCh, calcAgent.Name())
}

// Shared event consumption pattern for consistency.
func consume(eventsCh <-chan core.Event, errorsCh <-chan error, focus string) {
	for eventsCh != nil || errorsCh != nil {
		select {
		case ev, ok := <-eventsCh:
			if !ok {
				eventsCh = nil
				continue
			}
			if ev.Author == focus && ev.Content != nil {
				printParts(ev.Content.Parts)
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

func printParts(parts []core.Part) {
	for _, p := range parts {
		if tp, ok := p.(core.TextPart); ok {
			fmt.Println(tp.Text)
		}
	}
}

func newUserText(txt string) core.Content {
	return core.Content{Role: "user", Parts: []core.Part{core.TextPart{Text: txt}}}
}
