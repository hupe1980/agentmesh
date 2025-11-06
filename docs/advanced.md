---
layout: doc
title: Advanced Features
description: Explore checkpointing, time travel, human-in-the-loop, and other advanced graph capabilities.
permalink: /advanced/
hero:
  title: Advanced graph features
  description: Leverage checkpointing, time travel debugging, human-in-the-loop workflows, and more.
  primary_cta:
    label: Explore examples
    href: "https://github.com/hupe1980/agentmesh/tree/main/examples"
    external: true
  secondary_cta:
    label: Graph API →
    href: "https://pkg.go.dev/github.com/hupe1980/agentmesh/pkg/graph"
    external: true
sidebar:
  - title: Human-in-the-loop
    url: "#human-in-the-loop"
  - title: Message retention
    url: "#message-retention"
  - title: Retry policies
    url: "#retry-policies"
  - title: Subgraphs
    url: "#subgraphs"
---

## Checkpointing & Time Travel

AgentMesh provides comprehensive state persistence and debugging capabilities. For complete documentation including:

- Checkpoint lifecycle and automatic state saving
- Storage backends (Memory, SQL, DynamoDB) with trade-off analysis
- Time-travel debugging patterns
- Production considerations and cleanup strategies
- Recovery and resume workflows

See the **[Checkpointing Guide](/checkpointing/)** for detailed coverage.

**Quick Example**:

```go
import "github.com/hupe1980/agentmesh/pkg/checkpoint"

// Enable checkpointing
store := checkpoint.NewMemory()
compiled, _ := builder.Compile(
    graph.WithCheckpointStore(store),
    graph.WithCheckpointInterval(1),
)

// Execute with automatic checkpointing
results, _ := compiled.Invoke(ctx, messages,
    graph.WithThreadID("workflow-123"),
)

// Resume from checkpoint after failure
results, _ = compiled.InvokeFromCheckpoint(ctx, "workflow-123")
```

**Examples**: 
- [Checkpointing example](https://github.com/hupe1980/agentmesh/tree/main/examples/checkpointing)
- [Time-travel debugging example](https://github.com/hupe1980/agentmesh/tree/main/examples/time_travel)

---

## Human-in-the-loop {#human-in-the-loop}

Pause execution for human approval or input:

```go
builder.Node("request_approval", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
    // Request human input
    return &graph.NodeResult{
        Updates: map[string]any{
            "status": "awaiting_approval",
        },
        Interrupt: true,  // Pause execution
    }, nil
})

// Execution pauses here
results, _ := compiled.Invoke(ctx, messages)

// After human provides input, resume
results, _ = compiled.Resume(ctx, threadID, map[string]any{
    "approved": true,
})
```

See `examples/human_pause` for a complete workflow.

---

## Message retention {#message-retention}

Limit conversation history to prevent context overflow:

```go
// Keep only the last 10 messages
state := graph.NewGraphState(10)

builder := graph.NewBuilder()
builder.SetState(state)
```

Older messages are automatically pruned as new ones are added.

See `examples/message_retention` for details.

---

## Retry policies {#retry-policies}

Configure automatic retries for transient failures:

```go
import "time"

builder.Node("unreliable_api", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
    // ... call external API ...
}, graph.WithRetryPolicy(&graph.RetryPolicy{
    MaxAttempts:    3,
    InitialBackoff: 100 * time.Millisecond,
    MaxBackoff:     1 * time.Second,
    Multiplier:     2.0,
}))
```

The node will be retried with exponential backoff until it succeeds or reaches max attempts.

See `examples/retry` for various retry scenarios.
  o.Plugins = []core.Plugin{
    plugin.NewGlobalInstructions(&global),
    plugin.NewInputArtifactSaver(),
  }
})

runner := am.NewRunner(application)
defer runner.Close()
```

`plugin.NewGlobalInstructions` accepts any `*core.Instructions`, so you can construct provider-backed instructions (for example, from a database) and rely on the plugin to resolve and template them against the current session snapshot.

---

## Write your own plugin {#write-your-own-plugin}

Creating a plugin is as simple as implementing the `core.Plugin` interface. You can embed `plugin.Noop` to inherit default behaviors and override only the hooks you care about.

```go
import (
  "context"

  am "github.com/hupe1980/agentmesh"
  "github.com/hupe1980/agentmesh/core"
  "github.com/hupe1980/agentmesh/plugin"
)

type auditPlugin struct {
  plugin.Noop
}

func (p *auditPlugin) OnEvent(ctx context.Context, req core.RequestContext, ev *core.Event) (*core.Event, error) {
  // Forward a copy to your telemetry pipeline here.
  // Returning nil keeps the original event untouched.
  return nil, nil
}

// assume `agent` was created as shown above
application := am.NewApp("audited_app", agent, func(o *am.AppOptions) {
  o.Plugins = []core.Plugin{&auditPlugin{}}
})
```

You can override any combination of hooks—AgentMesh calls them at the appropriate lifecycle moments.

---
