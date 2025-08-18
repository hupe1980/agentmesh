// Package examples contains aggregated documentation for runnable subdirectory
// examples demonstrating AgentMesh capabilities.
//
// These examples showcase different usage patterns and integration approaches:
//
//   - basic_agent: Simple conversational LLM agent with AgentMesh facade
//   - weather_agent: Tool integration with custom weather lookup function
//   - multi_agent: Sequential workflow with state sharing between agents
//   - tool_usage: Mathematical calculations with step-by-step reasoning
//
// Each example is self-contained and includes:
//   - Environment setup (API keys, dependencies)
//   - Agent configuration and tool registration
//   - Event processing and response handling
//   - Error handling and timeout management
//
// To run an example:
//
//	export OPENAI_API_KEY="your-key-here"
//	go run examples/basic_agent/main.go
//
// These examples serve as both documentation and integration tests,
// demonstrating real-world usage patterns for different agent types
// and orchestration scenarios.
package examples
