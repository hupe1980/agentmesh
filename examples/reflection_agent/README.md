# Example: Reflection Agent

## Overview
Demonstrates the **Reflection Agent** - a composable wrapper that adds self-critique and refinement capabilities to ANY agent type (ReAct, RAG, Supervisor, custom). The reflection agent wraps another agent and iteratively improves its answers through self-critique loops.

## Key Concepts
- **Reflection Mode**: Agent reviews and improves its own outputs
- **Self-Critique Loop**: Automatic quality assessment and refinement
- **Iterative Refinement**: Multiple passes to improve answer quality
- **Meta-Reasoning**: Agent reasons about its own reasoning

## How It Works

```
1. Generate initial answer
   ↓
2. Critique the answer (reflection)
   ↓
3. Generate improved answer based on critique
   ↓
4. Repeat until max iterations or quality threshold met
```

## Configuration Options

```go
// Wrap ANY agent with reflection
baseAgent, _ := agent.NewReAct(model, agent.WithTools(tool))
reflectionAgent, _ := agent.NewReflection(
    baseAgent,                                    // The agent to wrap
    reflectionModel,                              // Model for critique
    agent.WithReflectionMaxIterations(3),         // Max refinement iterations
    agent.WithReflectionPromptTemplate("..."),    // Custom critique prompt
    agent.WithReflectionModelMiddleware(...),     // Middleware for reflection
)
```

**Key Advantage**: The reflection agent can wrap ANY agent type:
```go
// Wrap a ReAct agent
reactAgent, _ := agent.NewReAct(model, agent.WithTools(...))
reflected := agent.NewReflection(reactAgent, reflectionModel)

// Wrap a RAG agent
ragAgent, _ := agent.NewRAG(model, retriever)
reflected := agent.NewReflection(ragAgent, reflectionModel)

// Wrap a Supervisor agent
supervisor, _ := agent.NewSupervisor(model, agent.WithWorker(...))
reflected := agent.NewReflection(supervisor, reflectionModel)
```

## Prerequisites
```bash
export OPENAI_API_KEY="sk-..."
```

## Running
```bash
cd examples/reflection_agent
go run main.go
```

## Expected Output
```
=== ReAct Agent with Reflection Example ===

Question: Explain what recursion is in programming and provide a simple example.

Agent processing with reflection...
============================================================

--- Iteration 1 ---
[Initial answer about recursion]

🔍 Reflection feedback received
   (Agent is now refining its answer...)

--- Iteration 2 ---
[Improved answer with better explanation]

🔍 Reflection feedback received
   (Agent is now refining its answer...)

--- Iteration 3 ---
[Final refined answer]

============================================================

Final Answer:
[Best quality answer after reflections]

✅ Reflection example completed!
```

## When to Use Reflection

**Best for:**
- Complex reasoning tasks requiring deep thought
- Creative writing that benefits from editing
- Code generation with self-review
- Mathematical proofs needing verification
- Any task where quality improves with iteration

**Not needed for:**
- Simple factual questions
- Quick responses where first answer is sufficient
- Time-sensitive applications
- Tasks with clear right/wrong answers

## Performance Considerations

- **Token Usage**: Each reflection doubles the token count (critique + refinement)
- **Latency**: Multiple LLM calls increase response time
- **Quality**: Often produces significantly better answers
- **Cost**: More expensive due to additional model calls

## Advanced Usage

### Custom Reflection Prompt
```go
customPrompt := `Review this answer and identify:
1. Factual errors or inaccuracies
2. Missing important details
3. Clarity and organization issues
4. Ways to make it more helpful

Draft: {draft}

Provide specific, actionable feedback.`

agent.NewReAct(model,
    agent.WithReflection(true),
    agent.WithReflectionPrompt(customPrompt),
)
```

### Different Model for Reflection
```go
// Use a more capable model for critique
reflectionModel := openai.NewModel(
    openai.WithModel("gpt-4"),
)
reflectionExecutor := model.NewExecutor(reflectionModel)

agent.NewReAct(baseModel,
    agent.WithReflection(true),
    agent.WithReflectionExecutor(reflectionExecutor),
)
```

### Reflection with Tools
```go
// Reflection works seamlessly with tool calling
reactAgent, _ := agent.NewReAct(model,
    agent.WithTools(searchTool, calculatorTool),
)

reflectionAgent, _ := agent.NewReflection(
    reactAgent,
    reflectionModel,
    agent.WithReflectionMaxIterations(2),
)
// Agent can use tools AND reflect on the results!
```

## Implementation Details

The reflection feature adds a **reflection node** to the ReAct graph:

```
START → model → [tool | reflection | END]
          ↑         ↓
          └────────┘
```

**Flow:**
1. Model generates answer
2. If reflection enabled and not at max iterations:
   - Route to reflection node
   - Reflection node critiques the answer
   - Adds critique as system message
   - Routes back to model for refinement
3. Otherwise route to END

## Comparison with Basic ReAct

| Feature | Basic ReAct | With Reflection |
|---------|-------------|-----------------|
| **Speed** | Fast | Slower |
| **Cost** | Low | Higher |
| **Quality** | Good | Better |
| **Use Case** | Most tasks | Complex reasoning |
| **Token Usage** | ~1x | ~2-3x |

## Related Examples
- `basic_agent` - Simple ReAct without reflection
- `structured_output` - Constrained output format
- `supervisor_agent` - Multi-agent coordination

## References
- [ReAct Paper](https://arxiv.org/abs/2210.03629) - Original reasoning + acting pattern
- [Reflexion Paper](https://arxiv.org/abs/2303.11366) - Reflection for agents
- [Self-Refine Paper](https://arxiv.org/abs/2303.17651) - Iterative refinement
