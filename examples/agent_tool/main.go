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
	"github.com/hupe1980/agentmesh/tool"
)

// This example demonstrates using the AgentTool to run one agent from another.
// A SupervisorAgent is equipped with a GreeterAgent tool. The supervisor calls
// the tool with the requested greeting text, and the tool runs the greeter agent
// to produce the greeting.
func main() {
	if os.Getenv("OPENAI_API_KEY") == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}

	model := openai.NewModel(func(o *openai.Options) {
		o.Temperature = 0
	})

	// Inner agent that actually crafts the greeting
	greeter, err := agent.NewModelAgent("GreeterAgent", model, func(o *agent.ModelAgentOptions) {
		o.Instructions = agent.NewInstructionsFromText(
			"You are a warm greeter. The user message is the exact subject to greet (name or short phrase).\n" +
				"Constraints:\n" +
				"- Respond with exactly one sentence addressing the subject by name.\n" +
				"- Be friendly and polite.\n" +
				"- Do not ask follow-up questions or add extra sentences.\n" +
				"Output only the greeting text.",
		)
	})
	if err != nil {
		log.Fatalf("failed creating greeter agent: %v", err)
	}

	// Expose GreeterAgent as a tool
	greeterTool := tool.NewAgentTool(greeter)

	// Outer agent that decides when to invoke the greeter tool
	supervisor, err := agent.NewModelAgent("SupervisorAgent", model, func(o *agent.ModelAgentOptions) {
		o.Instructions = agent.NewInstructionsFromText(
			"You are a routing assistant. When the user asks to greet someone:\n" +
				"- Call the GreeterAgent tool.\n" +
				"- Pass ONLY the subject to greet (e.g., the person's name) in the '__arg1' field.\n" +
				"  Do NOT pass a full greeting sentence.\n" +
				"- After the tool returns, output EXACTLY the tool result text verbatim.\n" +
				"  Do NOT paraphrase, wrap in quotes, or add commentary.",
		)
		o.Tools = []core.Tool{greeterTool}
	})
	if err != nil {
		log.Fatalf("failed creating supervisor agent: %v", err)
	}

	r := runner.New("agent_tool_app", supervisor, func(o *runner.Options) {
		o.Logger = logging.NewSlogLogger(logging.LogLevelInfo, logging.LogFormatText, false)
	})
	defer func() { _ = r.Close() }()

	// Ask for a greeting; the supervisor should call the greeter tool.
	userParts := []core.Part{core.NewPartFromText("Greet Alice politely.")}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	runID, results, err := r.Run(ctx, "user1", "sess-agent-tool", userParts)
	if err != nil {
		log.Fatalf("invoke failed: %v", err)
	}

	fmt.Printf("=== Agent Tool Example [runID=%s] ===\n", runID)
	for res := range results {
		if res.Err != nil {
			log.Printf("error: %v", res.Err)
			continue
		}
		if ev := res.Event; ev != nil {
			printEvent(ev)
		}
	}
}

func printEvent(ev *core.Event) {
	for _, p := range ev.Parts {
		switch v := p.(type) {
		case *core.TextPart:
			fmt.Printf("\n[%s]\n%s\n", ev.Author, v.Text)
		case *core.FunctionCallPart:
			fmt.Printf("\n[%s -> function_call]\n%s %s\n", ev.Author, v.FunctionCall.Name, v.FunctionCall.Arguments)
		case *core.FunctionResponsePart:
			fmt.Printf("\n[%s -> function_response]\n%s => %v\n", ev.Author, v.FunctionResponse.Name, v.FunctionResponse.Response)
		}
	}
}
