# Supervisor Agent Example

This example demonstrates the **Supervisor Agent Pattern** using the `agent.NewSupervisorAgent()` template for simplified multi-agent coordination.

## Pattern Overview

The supervisor pattern delegates tasks to specialized worker agents based on the user's query. This example uses:

- **Supervisor Agent**: Routes questions to appropriate specialists
- **Worker Agents**: Math, History, and Code specialists
- **Tool-Based Handoffs**: Using `HandoffToAgent` under the hood

## Key Features

### 1. Simple Functional Options API

```go
supervisor, err := agent.NewSupervisorAgent(
	model,
	agent.WithWorker("math", "Math expert", mathAgent),
	agent.WithWorker("history", "History expert", historyAgent),
	agent.WithWorker("code", "Programming expert", codeAgent),
	agent.WithSupervisorSystemPrompt("Route to specialists"),
	agent.WithSupervisorMaxIterations(10),
	agent.WithWorkerContext(false),
	agent.WithWorkerRetries(2),
)
```

### 2. Automatic Tool Creation

The supervisor automatically creates `handoff_to_<worker>` tools for each worker agent.

### 3. Fresh Context per Task

By setting `IncludeContext: false`, each worker receives only the specific task, not the full conversation history.

## Running the Example

```bash
export OPENAI_API_KEY=sk-...
go run main.go
```

## Example Output

================================================================================
Query 1: What is the derivative of x^2 + 3x + 5?
================================================================================

👤 User:
   What is the derivative of x^2 + 3x + 5?

🎯 Supervisor Routing:
   → Delegating to: handoff_to_math
   → Task: Find the derivative of x^2 + 3x + 5

✨ Specialist Result:
   The derivative is 2x + 3
   

🤖 Response:
   The derivative is 2x + 3

## Comparison with Manual Setup

### Using Supervisor Template ✅

```go
supervisor, err := agent.NewSupervisorAgent(
	model,
	agent.WithWorker("math", "Math expert", mathAgent),
	agent.WithSupervisorMaxIterations(10),
)
```

### Manual Setup (Without Template)

```go
mathTool, err := tool.HandoffToAgent("math", "Math expert", mathAgent, 
	tool.WithContext(false), tool.WithRetries(2))
historyTool, err := tool.HandoffToAgent("history", "History expert", historyAgent,
	tool.WithContext(false), tool.WithRetries(2))

supervisor, err := agent.NewReActAgent(model,
	agent.WithTools(mathTool, historyTool),
	agent.WithSystemPrompt("Route to specialists"),
	agent.WithMaxIterations(10))
```

## Benefits

1. **Less Boilerplate**: No need to manually create handoff tools
2. **Cleaner Code**: Configuration in one place
3. **Consistent Patterns**: Standard supervisor setup
4. **Easy to Extend**: Just add workers to the config



## See Also

- [Agent Documentation](/docs/agents.md) - Learn about agent patterns
- [Tool Documentation](/docs/tools.md) - Understanding HandoffToAgent tools
- [Architecture Documentation](/docs/architecture.md) - Graph execution model

