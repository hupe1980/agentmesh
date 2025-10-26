package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/hupe1980/agentmesh/agent"
	"github.com/hupe1980/agentmesh/core"
)

type staticTool struct {
	name        string
	description string
}

func (t staticTool) Name() string        { return t.name }
func (t staticTool) Description() string { return t.description }
func (t staticTool) Parameters() map[string]any {
	return map[string]any{"type": "object"}
}
func (t staticTool) ProcessModelRequest(ctx context.Context, toolCtx core.ToolContext, req *core.ModelRequest) error {
	return nil
}
func (t staticTool) Call(ctx context.Context, toolCtx core.ToolContext, args string) (any, error) {
	return "stub-result", nil
}

type toolAgent struct {
	*agent.FuncAgent
	tools []core.Tool
}

func newToolAgent(name, description string, tools []core.Tool) *toolAgent {
	run := func(ctx context.Context, reqCtx core.RequestContext, queue core.EventWriter) error {
		return nil
	}

	fa := agent.NewFuncAgent(name, run, func(o *agent.FuncAgentOptions) {
		o.Description = description
	})

	return &toolAgent{FuncAgent: fa, tools: tools}
}

func (a *toolAgent) Tools() []core.Tool { return a.tools }

func main() {
	researcher := newToolAgent("ResearchAgent", "Gathers facts from internal knowledge base", []core.Tool{
		staticTool{name: "search_docs", description: "Search the document index"},
	})

	writer := newToolAgent("WriterAgent", "Drafts the report using gathered insights", []core.Tool{
		staticTool{name: "compose_paragraph", description: "Draft a paragraph with given outline"},
	})

	reviewer := agent.NewFuncAgent("ReviewAgent", func(ctx context.Context, reqCtx core.RequestContext, queue core.EventWriter) error {
		return nil
	}, func(o *agent.FuncAgentOptions) {
		o.Description = "Ensures the final report meets quality expectations"
	})

	workflow := agent.NewSequentialAgent("QuarterlyReport", []core.Agent{researcher, writer, reviewer}, func(o *agent.SequentialAgentOptions) {
		o.Description = "Coordinates research, writing, and review"
	})

	chart, err := agent.Flowchart(workflow,
		agent.WithDirection("LR"),
		agent.WithDescriptions(true),
		agent.WithTools(true),
	)
	if err != nil {
		log.Fatalf("failed to render flowchart: %v", err)
	}

	fmt.Println("Mermaid flowchart definition:")
	fmt.Println()
	fmt.Println(chart)

	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		log.Fatal("failed to determine source directory")
	}

	outputPath := filepath.Join(filepath.Dir(sourceFile), "flowchart.mmd")

	if err := os.WriteFile(outputPath, []byte(chart), 0o600); err != nil {
		log.Fatalf("failed to write flowchart.mmd: %v", err)
	}

	fmt.Printf("\nWrote %s — open it in your favorite Mermaid viewer.\n", outputPath)
}
