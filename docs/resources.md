---
layout: doc
title: Resources
permalink: /resources/
hero:
  title: Keep exploring AgentMesh
  description: Find examples, roadmap discussions, and contributor guides.
  primary_cta:
    label: Browse the repository
    href: "https://github.com/hupe1980/agentmesh"
    external: true
  secondary_cta:
    label: Open an issue →
    href: "https://github.com/hupe1980/agentmesh/issues"
    external: true
sidebar:
  - title: Essential links
    url: "#essential-links"
  - title: Community & feedback
    url: "#community-feedback"
  - title: Local development tips
    url: "#local-development-tips"
  - title: Related reading
    url: "#related-reading"
---

## Essential links

- **Repository** – [github.com/hupe1980/agentmesh](https://github.com/hupe1980/agentmesh)
- **API reference** – [pkg.go.dev/github.com/hupe1980/agentmesh](https://pkg.go.dev/github.com/hupe1980/agentmesh)
- **Examples** – [examples directory](https://github.com/hupe1980/agentmesh/tree/main/examples)
- **Changelog** – [GitHub releases](https://github.com/hupe1980/agentmesh/releases)

---

## Community & feedback

- File issues, feature requests, or questions via [GitHub Issues](https://github.com/hupe1980/agentmesh/issues).
- Join discussions and propose ideas through pull requests—start with [CONTRIBUTING.md](https://github.com/hupe1980/agentmesh/blob/main/CONTRIBUTING.md).
- Track upcoming work or propose new items on the [project roadmap](https://github.com/hupe1980/agentmesh/issues?q=is%3Aopen+label%3Aroadmap).

---

## Local development tips

- Lint and test quickly using `just lint`, `just test`, or `just check`.
- Run all Go tests with race detection using `go test ./... -race`.
- Preview documentation updates locally with `just docs-serve`.

---

## Related reading

- [OpenTelemetry example](https://github.com/hupe1980/agentmesh/tree/main/examples/opentelemetry) – combine structured logging, metrics, and tracing.
- [Tool usage example](https://github.com/hupe1980/agentmesh/tree/main/examples/tool_usage) – define strongly typed tools with JSON Schema validation.
- [Transfer agent example](https://github.com/hupe1980/agentmesh/tree/main/examples/transfer_agent) – hand off control between agents.
