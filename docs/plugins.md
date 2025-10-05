---
layout: doc
title: Plugins
description: Intercept runner, agent, model, and tool lifecycles with reusable hooks.
permalink: /plugins/
hero:
  title: Extend AgentMesh with plugins
  description: Observe, mutate, or short-circuit requests at any lifecycle stage using pluggable hooks.
  primary_cta:
    label: Register a plugin
    href: "#register-plugins"
  secondary_cta:
    label: Plugin API →
    href: "https://pkg.go.dev/github.com/hupe1980/agentmesh/core#Plugin"
    external: true
sidebar:
  - title: Why plugins?
    url: "#why-plugins"
  - title: Register plugins
    url: "#register-plugins"
  - title: Built-in plugins
    url: "#built-in-plugins"
  - title: Write your own plugin
    url: "#write-your-own-plugin"
---

## Why plugins? {#why-plugins}

Plugins are the extensibility backbone of AgentMesh. They run alongside every invocation and can:

- inspect or rewrite user input before the run starts;
- short-circuit agents, models, or tools with cached results;
- log or redact streaming events as they flow to the client;
- implement retries, fallbacks, analytics, and more.

Every plugin implements [`core.Plugin`](https://pkg.go.dev/github.com/hupe1980/agentmesh/core#Plugin), a rich interface that exposes hooks such as `BeforeRun`, `OnEvent`, `BeforeModel`, and `AfterTool`.

---

## Register plugins on an application {#register-plugins}

Attach plugins when you construct your application. The façade lets you set them through `am.AppOptions` so every runner derived from the app shares the same plugin set.

```go
import (
  am "github.com/hupe1980/agentmesh"
  "github.com/hupe1980/agentmesh/core"
  "github.com/hupe1980/agentmesh/model/openai"
  "github.com/hupe1980/agentmesh/plugin"
)

model := openai.NewModel()
agent, err := am.NewModelAgent("with_plugins", model)
if err != nil {
  panic(err)
}

application := am.NewApp("with_plugins", agent, func(o *am.AppOptions) {
  o.Plugins = []core.Plugin{
    plugin.NewInputArtifactSaver(), // persist raw uploads as artifacts
    plugin.NewNoop(),               // placeholder demonstrating multiple plugins
  }
})

runner := am.NewRunner(application)
```

Plugins execute in registration order. In the example above, the input artifact saver runs before the no-op plugin.

---

## Built-in plugins {#built-in-plugins}

AgentMesh ships several ready-to-use plugins:

- **`plugin.NewInputArtifactSaver()`** – scans incoming user parts for raw file payloads (bytes/base64), persists them as artifacts, and replaces them with `artifact:` URIs so downstream tools receive lightweight references. Pair it with any artifact store; the in-memory store works out of the box for development.
- **`plugin.NewGlobalInstructions()`** – prepends a shared instruction block before every model invocation. Useful for enforcing policy language or injecting guardrails without touching each agent.
- **`plugin.NewNoop()`** – a pass-through implementation handy for tests or as an embedding base class.

Example: apply global instructions to every call while still allowing agents to set their own prompts.

```go
import (
  am "github.com/hupe1980/agentmesh"
  "github.com/hupe1980/agentmesh/core"
  "github.com/hupe1980/agentmesh/model/openai"
  "github.com/hupe1980/agentmesh/plugin"
)

model := openai.NewModel()
agent, err := am.NewModelAgent("guarded", model, func(o *am.ModelAgentOptions) {
  o.Instructions = am.NewInstructionsFromText("Answer the user's question succinctly.")
})
if err != nil {
  panic(err)
}

global := core.NewInstructionsFromText("You must follow corporate policy ACME-42.")

application := am.NewApp("policy_app", agent, func(o *am.AppOptions) {
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
