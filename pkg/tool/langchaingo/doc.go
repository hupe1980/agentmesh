// Package langchaingo provides adapters for using langchaingo tools
// (github.com/tmc/langchaingo/tools) within AgentMesh workflows.
//
// This package allows you to leverage the rich ecosystem of langchaingo
// tools in your AgentMesh agents without modification.
//
// Example:
//
//	import (
//	    "context"
//	    "github.com/hupe1980/agentmesh/pkg/tool/langchaingo"
//	    lc "github.com/tmc/langchaingo/tools"
//	)
//
//	// Create a langchaingo tool
//	calculator := lc.Calculator{}
//
//	// Wrap it for use in AgentMesh
//	tool, err := langchaingo.NewTool(calculator)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Or customize the name and description
//	tool, err := langchaingo.NewTool(
//	    calculator,
//	    langchaingo.WithName("my_calculator"),
//	    langchaingo.WithDescription("Custom calculator"),
//	)
//
//	// Use it in your agent
//	agent := agent.NewReActAgent(model, []tool.Tool{tool})
package langchaingo
