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

// multi_agent demonstrates a sequential workflow of specialized agents sharing state.
func main() {
	if os.Getenv("OPENAI_API_KEY") == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}

	model := openai.NewModel()

	// Current date for time-bounded instructions
	today := time.Now().Format("2006-01-02")

	// Step 1: Research specialist
	researchAgent, err := agent.NewModelAgent("ResearchAgent", model, func(o *agent.ModelAgentOptions) {
		o.Instructions = agent.NewInstructionsFromText(fmt.Sprintf(
			`
You are a senior research analyst focused on EU AI regulation. Your task is to research the latest news and official updates about Generative AI regulation in the EU as of %s.

Requirements:
- Prioritize EU AI Act, implementing acts, guidance, enforcement timelines, and national implementations since 2024.
- Prefer credible sources: EU institutions (Commission, Parliament, Council), EDPB, ENISA, national DPAs, reputable outlets, and major think tanks.
- Provide 3-5 key findings; each finding must include a 1-2 sentence summary and a citation with source name, URL, and date.
- Include a concise timeline (3-6 dated milestones) and 3-5 open questions.
- Avoid speculation. If unknown or conflicting, say "Unknown" or note the contradiction.

Output format (use exactly these section headers):
=== RESEARCH ===
Overview:

Key Findings:
- [Finding N] … (Source: Name — URL — YYYY-MM-DD)

Timeline:
- YYYY-MM-DD — Event

Open Questions:
- …

Sources:
- Name — URL (YYYY-MM-DD)
=== END ===
`, today))
		o.OutputKey = "research_data"
	})
	if err != nil {
		log.Fatalf("failed creating research agent: %v", err)
	}

	// Step 2: Analysis specialist (consumes research_data)
	analysisAgent, err := agent.NewModelAgent("AnalysisAgent", model, func(o *agent.ModelAgentOptions) {
		o.Instructions = agent.NewInstructionsFromText(
			`
You are a policy and risk analysis specialist. Analyze the following research to derive actionable insights:

{{ .research_data }}

Requirements:
- Extract 3-5 top themes with one-sentence takeaways; add [confidence: high|medium|low] to each.
- Assess impacts for: Startups, Enterprises, Open-source, Regulators (1-2 bullets each).
- List key risks/uncertainties and where evidence is weak or contradictory.
- Provide 3-5 recommendations labeled Short-term (S), Medium-term (M), or Long-term (L).
- Do not invent URLs; reference sources by name if needed, or say "Unknown".

Output format:
=== ANALYSIS ===
Executive Summary:
...

Key Insights:
1. ... [confidence: ]
2. ... [confidence: ]

Impacts:
- Startups: ...
- Enterprises: ...
- Open-source: ...
- Regulators: ...

Risks & Uncertainties:
- ...

Recommendations:
- (S) ...
- (M) ...
- (L) ...
=== END ===
`,
		)
		o.OutputKey = "analysis_results"
	})
	if err != nil {
		log.Fatalf("failed creating analysis agent: %v", err)
	}

	// Step 3: Report writer (consumes analysis_results)
	reportAgent, err := agent.NewModelAgent("ReportAgent", model, func(o *agent.ModelAgentOptions) {
		o.Instructions = agent.NewInstructionsFromText(
			`
You are a concise technical report writer. Using the analysis below, craft a clear, executive-ready report in Markdown.

{{ .analysis_results }}

Requirements:
- Write a descriptive title and a 4-6 sentence summary.
- Organize the body with headings: Background, Current State, Impacts, Recommendations.
- Keep recommendations actionable (who/what/when). Avoid speculation.
- Include a "Sources" section at the end; do not duplicate raw URLs in the body.

Output format (Markdown):
# <Title>

## Summary
...

## Background
...

## Current State
...

## Impacts
...

## Recommendations
...

## Sources
- Name — URL (YYYY-MM-DD)
`,
		)
	})
	if err != nil {
		log.Fatalf("failed creating report agent: %v", err)
	}

	// Sequential workflow: Research → Analysis → Report
	workflow := agent.NewSequentialAgent("MultiAgent", []core.Agent{researchAgent, analysisAgent, reportAgent})

	r := runner.New("multi_agent_app", workflow, func(o *runner.Options) {
		o.Logger = logging.NewSlogLogger(logging.LogLevelInfo, logging.LogFormatText, false)
	})
	defer func() {
		_ = r.Close()
	}()

	userParts := []core.Part{core.NewPartFromText(
		"Research and summarize the latest news about Generative AI regulation in the EU.",
	)}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	runID, results, err := r.Run(ctx, "user1", "sess1", userParts)
	if err != nil {
		log.Fatalf("invoke failed: %v", err)
	}

	fmt.Printf("=== Multi-Agent Workflow [runID=%s] ===\n", runID)
	consume(results)
}

func consume(results <-chan core.RunResult) {
	for res := range results {
		if res.Err != nil {
			log.Printf("error: %v", res.Err)
			continue
		}

		for _, part := range res.Event.Parts {
			if tp, ok := part.(*core.TextPart); ok {
				fmt.Printf("\n[%s]\n%s\n", res.Event.Author, tp.Text)
			}
		}
	}
}
