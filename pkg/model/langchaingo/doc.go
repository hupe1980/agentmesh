// Package langchaingo provides an adapter for using LangChainGo
// (github.com/tmc/langchaingo) models within AgentMesh workflows.
//
// This adapter enables integration with LangChainGo's 50+ model providers
// (OpenAI, Anthropic, Google AI, Cohere, local models, etc.) while using
// the AgentMesh model interface for agents and graph execution.
//
// Example usage:
//
//	import (
//	    "github.com/hupe1980/agentmesh/pkg/model/langchaingo"
//	    "github.com/tmc/langchaingo/llms/openai"
//	)
//
//	// Create a LangChainGo model
//	llm, err := openai.New(openai.WithModel("gpt-4"))
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Wrap it as an AgentMesh model
//	model, _ := langchaingo.NewModel(llm)
//
//	// Use with AgentMesh agents
//	agent, err := agent.NewReAct(model, tools)
package langchaingo
