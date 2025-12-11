# Blog Writer Example

This example demonstrates a multi-agent blog writing system using the AgentMesh Supervisor pattern. It shows how to coordinate multiple specialized AI agents to create high-quality, SEO-optimized blog posts.

## What It Shows

- Creating a supervisor to orchestrate multiple worker agents
- Building specialized agents for different tasks (keywords, headlines, writing, editing)
- Coordinating a multi-step content creation workflow
- Using `agent.NewSupervisor()` with `agent.WithWorker()` options
- Passing context between agents in a pipeline

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                   Blog Writer Supervisor                 │
│                                                         │
│  Coordinates workflow and passes context between agents │
└─────────────────────────────────────────────────────────┘
                           │
     ┌─────────────────────┼─────────────────────┐
     │                     │                     │
     ▼                     ▼                     ▼
┌─────────┐         ┌─────────┐          ┌─────────┐
│Keywords │ ──────► │Headlines│ ───────► │ Writer  │
│  Agent  │         │  Agent  │          │  Agent  │
└─────────┘         └─────────┘          └─────────┘
                                              │
                                              ▼
                                         ┌─────────┐
                                         │ Editor  │
                                         │  Agent  │
                                         └─────────┘
                                              │
                                              ▼
                                        Final Blog Post
```

## Agents

### 1. Keyword Generator Agent
- Analyzes the blog topic
- Generates primary keywords, secondary keywords, and long-tail phrases
- Considers search intent and SEO optimization

### 2. Headline Creator Agent
- Creates 5 headline options in different styles:
  - How-to/Tutorial format
  - Listicle format
  - Question format
  - Provocative format
  - Data-driven format
- Evaluates each headline for clickability, SEO, and clarity
- Selects the best headline

### 3. Content Writer Agent
- Writes a comprehensive blog post (1500-2000 words)
- Follows best practices:
  - Attention-grabbing hook
  - Clear structure with H2/H3 headers
  - Short paragraphs for scannability
  - Statistics and examples
  - Call-to-action ending
- Outputs in Markdown format

### 4. Editor/Reviewer Agent
- Reviews the draft for quality
- Scores on readability, engagement, SEO, and quality
- Provides specific improvement suggestions
- Approves or requests revisions

## Running the Example

```bash
export OPENAI_API_KEY="sk-..."
go run main.go
```

## Progress Middleware

This example includes a custom progress middleware that shows which agent is currently working:

```go
func progressMiddleware() graph.Middleware[message.Message] {
    return func(next graph.NodeFunc[message.Message]) graph.NodeFunc[message.Message] {
        return func(ctx context.Context, scope graph.Scope[message.Message]) (*graph.Command, error) {
            nodeName := scope.NodeName()
            messages := message.GetMessages(scope)

            switch nodeName {
            case "model":
                fmt.Printf("🤖 Thinking...\n")
            case "tool":
                // Find tool calls in the last AI message
                for i := len(messages) - 1; i >= 0; i-- {
                    if aiMsg, ok := messages[i].(*message.AIMessage); ok && len(aiMsg.ToolCalls) > 0 {
                        for _, tc := range aiMsg.ToolCalls {
                            fmt.Printf("🔧 Calling: %s\n", tc.Name)
                        }
                        break
                    }
                }
            }

            result, err := next(ctx, scope)
            return result, err
        }
    }
}
```

The middleware is applied via `agent.WithGraphMiddleware(progressMiddleware())`.

## Expected Output

```
================================================================================
🚀 AgentMesh Blog Writer
================================================================================

Topic: How AI is transforming software development productivity in 2025

📝 Starting blog generation...
────────────────────────────────────────────────────────────────
🤖 Thinking...
🔑 Delegating to: Keywords Agent
   ✅ Done (5.734s)
🤖 Thinking...
📰 Delegating to: Headlines Agent
   ✅ Done (6.961s)
🤖 Thinking...
✍️  Delegating to: Writer Agent
   ✅ Done (12.456s)
🤖 Thinking...
📝 Delegating to: Editor Agent
   ✅ Done (8.123s)
🤖 Thinking...

📄 GENERATED BLOG POST:
============================================================

# How AI Is Revolutionizing Developer Productivity in 2025

*The future of coding is here, and it's powered by artificial intelligence.*

---

In 2025, artificial intelligence has become an indispensable partner for software 
developers worldwide. What was once a novelty has transformed into a fundamental 
shift in how we write, review, and deploy code...

## The Rise of AI-Powered Development

...

============================================================
✅ Blog generation complete!
📊 Approximate word count: 1847
```

## Key Components

### Supervisor Setup

```go
return agent.NewSupervisor(
    model,
    agent.WithWorker("keywords", "SEO expert...", keywordAgent),
    agent.WithWorker("headlines", "Headline specialist...", headlineAgent),
    agent.WithWorker("writer", "Content writer...", writerAgent),
    agent.WithWorker("editor", "Senior editor...", editorAgent),
    agent.WithInstructions(supervisorPrompt),
    agent.WithMaxIterations(15),
    agent.WithWorkerRetries(2),
)
```

### Worker Agent Creation

```go
func createWriterAgent() (*message.Graph, error) {
    model := openai.NewModel()
    
    return agent.NewReAct(
        model,
        agent.WithInstructions(`You are an expert content writer...`),
        agent.WithMaxIterations(5),
    )
}
```

### Execution

```go
// Create input message
input := []message.Message{
    message.NewHumanMessageFromText("Write a blog post about: " + topic),
}

// Run the workflow
results, err := graph.Collect(blogWriter.Run(ctx, input))
```

## Extending the Example

### Add Research Agent
Add a web research agent using MCP tools or web search APIs:

```go
researchAgent, _ := agent.NewReAct(
    model,
    agent.WithTools(webSearchTool, urlFetchTool),
    agent.WithInstructions("Research the topic thoroughly..."),
)

agent.WithWorker("research", "Web researcher...", researchAgent),
```

### Add Image Generation
Add a hero image generator using DALL-E or GPT-4o:

```go
imageAgent, _ := agent.NewReAct(
    model,
    agent.WithTools(dalleImageTool),
    agent.WithInstructions("Generate a hero image..."),
)

agent.WithWorker("image", "Image generator...", imageAgent),
```

### Add Plagiarism Checking
Add an originality checker:

```go
plagiarismAgent, _ := agent.NewReAct(
    model,
    agent.WithTools(copyscapeTool),
    agent.WithInstructions("Check content originality..."),
)

agent.WithWorker("plagiarism", "Originality checker...", plagiarismAgent),
```

## Configuration Options

| Option | Description | Default |
|--------|-------------|---------|
| `WithMaxIterations` | Maximum supervisor iterations | 15 |
| `WithWorkerRetries` | Retries per worker on failure | 2 |
| `WithInstructions` | Supervisor system prompt | Required |

## Related Examples

- [supervisor_agent](../supervisor_agent) - Basic supervisor pattern
- [streaming](../streaming) - Real-time streaming output
- [basic_agent](../basic_agent) - Simple ReAct agent with tools
