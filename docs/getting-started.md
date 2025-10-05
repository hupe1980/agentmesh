---
layout: doc
title: Getting Started
permalink: /getting-started/
hero:
  title: Build with AgentMesh in minutes
  description: Install the library, run the quick start, and keep building with production-ready tooling.
  primary_cta:
    label: Install the module
    href: "#install"
  secondary_cta:
    label: Browse examples →
    href: "https://github.com/hupe1980/agentmesh/tree/main/examples"
    external: true
sidebar:
  - title: Install
    url: "#install"
  - title: Run the quick start
    url: "#run-the-quick-start"
  - title: Explore examples
    url: "#explore-examples"
  - title: Local docs preview
    url: "#local-docs-preview"
  - title: Next steps
    url: "#next-steps"
---

## Install {#install}

```bash
go get github.com/hupe1980/agentmesh
```

> Requires Go 1.24+ and appropriate model credentials (for example, `OPENAI_API_KEY`).

---

## Run the quick start

1. Export your model credential (OpenAI shown below).
2. Execute the basic agent example to stream a response end-to-end.

```bash
export OPENAI_API_KEY="your-key"
go run ./examples/basic_agent/main.go
```

You should see partial and final responses printed to the terminal, demonstrating the default runner, flow selection, and event streaming.

Prefer wiring the pieces manually? Use the façade constructors directly:

```go
import (
  "log"

  am "github.com/hupe1980/agentmesh"
  "github.com/hupe1980/agentmesh/model/openai"
)

model := openai.NewModel()

agent, err := am.NewModelAgent("basic_agent", model, func(o *am.ModelAgentOptions) {
  o.Instructions = am.NewInstructionsFromText("You are a concise assistant.")
})
if err != nil {
  log.Fatalf("failed to build agent: %v", err)
}

application := am.NewApp("basic_agent_app", agent)
r := am.NewRunner(application)
// ...
```

---

## Explore examples

The repository ships multiple ready-to-run apps covering tools, transfers, multi-agent orchestration, and observability. A few highlights:

- `examples/tool_usage` – model-driven tool calling with JSON Schema validation.
- `examples/multi_agent` – sequential + parallel composition patterns.
- `examples/opentelemetry` – structured logging, metrics, and tracing via OpenTelemetry.

Use `go run ./examples/<dir>/main.go` to try any example.

---

## Local docs preview

The devcontainer (or a local Ruby install) includes everything you need to iterate on this documentation site.

```bash
cd docs
bundle config set --local path 'vendor/bundle'
bundle install
bundle exec jekyll serve --livereload --host 0.0.0.0 --config _config.yml,_config.dev.yml
```

Prefer a single command in the devcontainer? Run `just docs-serve` from the repo root.

Once the server is running, open `http://localhost:4000` for the rendered docs with live reload.

---

## Next steps

- Learn how orchestration primitives compose in the [Agents guide](/agents/).
- Wire up tools and toolsets (including MCP) in the [Tools guide](/tools/).
- Understand the runtime internals in [Architecture](/architecture/).
