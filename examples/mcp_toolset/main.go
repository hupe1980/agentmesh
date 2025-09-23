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
	mcptool "github.com/hupe1980/agentmesh/tool/mcp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// simple typed input/output for our MCP tool
// The SDK will auto-generate JSON schemas for these.
type sumInput struct {
	A float64 `json:"a" jsonschema:"first operand"`
	B float64 `json:"b" jsonschema:"second operand"`
}

type sumOutput struct {
	Result float64 `json:"result" jsonschema:"sum of a and b"`
}

// sumTool implements a typed MCP tool handler using mcp.AddTool
func sumTool(_ context.Context, _ *mcp.CallToolRequest, in sumInput) (*mcp.CallToolResult, sumOutput, error) {
	return nil, sumOutput{Result: in.A + in.B}, nil
}

func newInMemoryMCP() *mcp.InMemoryTransport {
	// Create paired in-memory transports
	clientTransport, serverTransport := mcp.NewInMemoryTransports()

	// Server with a single tool
	server := mcp.NewServer(&mcp.Implementation{Name: "demo-mcp-server", Version: "v1.0.0"}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "sum",
		Description: "Add two numbers",
	}, sumTool)

	// Start serving in a background goroutine; will run until client disconnects
	go func() {
		if err := server.Run(context.Background(), serverTransport); err != nil {
			log.Printf("MCP server terminated: %v", err)
		}
	}()

	return clientTransport
}

func main() {
	if os.Getenv("OPENAI_API_KEY") == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}

	// 1) Build in-memory MCP server + paired transports
	clientTransport := newInMemoryMCP()

	// 2) Create a Toolset that will list tools from the MCP server
	ts := mcptool.NewToolset(mcptool.NewInMemorySessionFactory(clientTransport))

	// 3) Build an LLM agent and attach the MCP toolset
	model := openai.NewModel(func(o *openai.Options) { o.Temperature = 0 })

	llmAgent, err := agent.NewModelAgent("MCPAgent", model, func(o *agent.ModelAgentOptions) {
		o.Instructions = agent.NewInstructionsFromText("You can call the 'sum' tool to add two numbers. When asked to add, call the tool and return the numeric result.")
		o.Toolsets = []core.Toolset{ts}
	})
	if err != nil {
		log.Fatalf("failed creating agent: %v", err)
	}

	// 4) Run a sample conversation where the agent should invoke the MCP tool
	r := runner.New("mcp_example_app", llmAgent, func(o *runner.Options) {
		o.Logger = logging.NewSlogLogger(logging.LogLevelInfo, logging.LogFormatText, false)
	})

	defer func() { _ = r.Close() }()
	defer func() { _ = ts.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	parts := []core.Part{core.NewPartFromText("Please add 3.5 and 4.25")}
	runID, results, err := r.Run(ctx, "user1", "sess1", parts)
	if err != nil {
		log.Fatalf("run failed: %v", err)
	}

	fmt.Printf("=== MCP In-Memory Example [runID=%s] ===\n", runID)
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
			fmt.Printf("\n[%s]\n%s\n", ev.Author, v.Text)
		case *core.FunctionCallPart:
			fmt.Printf("\n[%s -> function_call]\n%s %s\n", ev.Author, v.FunctionCall.Name, v.FunctionCall.Arguments)
		case *core.FunctionResponsePart:
			fmt.Printf("\n[%s -> function_response]\n%s => %v\n", ev.Author, v.FunctionResponse.Name, v.FunctionResponse.Response)
		}
	}
}
