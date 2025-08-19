package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/hupe1980/agentmesh"
	"github.com/hupe1980/agentmesh/agent"
	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/logging"
	"github.com/hupe1980/agentmesh/model/openai"
	"github.com/hupe1980/agentmesh/tool"
)

// multi_agent demonstrates a sequential workflow of specialized agents sharing state.
func main() {
	if os.Getenv("OPENAI_API_KEY") == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}

	model := openai.NewModel()

	researchAgent := agent.NewModelAgent("ResearchAgent", model, func(o *agent.ModelAgentOptions) {
		o.Instruction = agent.NewInstructionFromText("You are a research specialist. Analyze topics and provide comprehensive research findings. Store your findings in the 'research_data' output key.")
		o.OutputKey = "research_data"
	})

	analysisAgent := agent.NewModelAgent("AnalysisAgent", model, func(o *agent.ModelAgentOptions) {
		o.Instruction = agent.NewInstructionFromText("You are an analysis specialist. Take research data and perform deep analysis. Use the research_data from session state. Store analysis in 'analysis_results'.")
		o.OutputKey = "analysis_results"
	})
	analysisAgent.RegisterTool(tool.NewStateManagerTool())

	reportAgent := agent.NewModelAgent("ReportAgent", model, func(o *agent.ModelAgentOptions) {
		o.Instruction = agent.NewInstructionFromText("You are a report writer. Use analysis_results from session state to craft a concise report.")
	})
	reportAgent.RegisterTool(tool.NewStateManagerTool())

	workflow := agent.NewSequentialAgent("MultiAgent", researchAgent, analysisAgent, reportAgent)

	mesh := agentmesh.New(workflow, func(o *agentmesh.Options) {
		o.Logger = logging.NewSlogLogger(logging.LogLevelInfo, "text", false)
	})

	userContent := newUserText("Research and analyze the impact of artificial intelligence on modern education systems.")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	_, eventsCh, errorsCh, err := mesh.Invoke(ctx, "sess1", userContent)
	if err != nil {
		log.Fatalf("invoke failed: %v", err)
	}

	fmt.Println("=== Multi-Agent Workflow ===")
	for eventsCh != nil || errorsCh != nil {
		select {
		case ev, ok := <-eventsCh:
			if !ok {
				eventsCh = nil
				continue
			}
			if ev.Content == nil {
				continue
			}
			printLabeled(ev)
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

func printLabeled(ev core.Event) {
	for _, part := range ev.Content.Parts {
		if tp, ok := part.(core.TextPart); ok {
			label := ev.Author
			fmt.Printf("\n[%s]\n%s\n", label, tp.Text)
		}
	}
}

// Shared helpers (duplicated minimal subset for readability; could import from a util package)
func newUserText(txt string) core.Content {
	return core.Content{Role: "user", Parts: []core.Part{core.TextPart{Text: txt}}}
}
