# Contributing to AgentMesh

Thanks for your interest in AgentMesh. This guide explains how to develop, test, and propose changes in a consistent, low-friction way.

---

## Quick Start

- Go >= 1.24
- Clone the repo and install dev tools:
  - golangci-lint: `just install-deps` or `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest`
- Verify everything:
  - `just test` (or `go test ./...`)
  - `just test-race` (or `go test ./... -race`)
  - `just lint`
  - `just cover` to generate HTML coverage

Key examples:
- Basic agent: [examples/basic_agent/main.go](examples/basic_agent/main.go)
- Tools: [examples/tool_usage/main.go](examples/tool_usage/main.go)
- Multi-agent: [examples/multi_agent/main.go](examples/multi_agent/main.go)
- Transfers: [examples/transfer_agent/main.go](examples/transfer_agent/main.go)

---

## Project Philosophy

- Minimal, stable core: Build around small contracts like [`core.Agent`](core/agent.go) and [`model.Model`](model/model.go).
- Determinism: Prefer stable ordering and predictable side-effects (good for tests and reliability).
- Streaming first: Emit partials early and often; minimize buffering.
- Safe tools: Validate inputs, isolate execution, honor context, and surface clear errors.
- Provider-agnostic: Adapters expose capabilities via [`model.Info`](model/model.go).

---

## Repository Structure (Condensed)

- Core contracts: [core/](core/)
- Runner orchestration: [runner/](runner/)
- Agents (model, sequential, parallel, loop): [agent/](agent/)
- Flows & processors: [flow/](flow/) (see [flow/processors.go](flow/processors.go))
- Tools and helpers: [tool/](tool/)
- Models (OpenAI, Anthropic): [model/openai/openai.go](model/openai/openai.go), [model/anthropic/anthropic.go](model/anthropic/anthropic.go)
- In-memory stores: [memory/](memory/), [artifact/](artifact/), [session/](session/)
- Logging & metrics: [logging/](logging/), [metrics/](metrics/)
- Examples: [examples/](examples/)
- CI & release: [.github/workflows/build.yml](.github/workflows/build.yml), [.goreleaser.yml](.goreleaser.yml)
- Lint config: [.golangci.yml](.golangci.yml)
- Project docs: [README.md](README.md), [TODO.md](TODO.md), this file

---

## Tooling and Commands

Using Just (preferred):
- `just` to list commands
- `just test` / `just test-race` / `just test-verbose`
- `just test-pkg agent` to run a package
- `just lint`
- `just cover` (HTML) and `just cover-summary` (by-package summary)
- `just check` (tests + lint + coverage)
- `just dev` (quick test + lint)
- `just clean` (remove coverage artifacts)

Plain Go:
- `go test ./... -race`
- `golangci-lint run --config .golangci.yml`

---

## Coding Standards

General
- go fmt + go vet clean before pushing.
- Group imports: std | third-party | internal.
- Use context.Context as the first parameter for call paths that can block or cancel.
- Keep files small and focused; avoid giant "god" objects.

Errors
- Prefix with domain (`artifact:`, `memory:`, `model/openai:`).
- Wrap with `%w` for propagation.
- Provide sentinel errors in the relevant package when needed.

Concurrency
- Always honor cancellation; prefer short `select` over blocking calls.
- Document channel buffer sizes and goroutine lifetimes.
- Avoid unbounded concurrency; bound with semaphores where applicable.

Logging and Metrics
- Prefer structured logs (see [logging/](logging/)).
- Add light metrics hooks only where they inform user-facing performance.

Determinism
- Avoid map-iteration-dependent behavior in outputs.
- Sort slices returned from stores where order matters to callers/tests.

---

## Events, Content, and Orchestration

- Agents emit streaming events and final outputs; keep mutation after emission to a minimum.
- Use explicit actions in events (e.g., state deltas, transfers) rather than side channels.
- Keep flows simple; prefer composition of agents (sequential/parallel/loop) to bespoke control logic.

Reference types and entry points:
- Agent interface: [`core.Agent`](core/agent.go)
- Model agent constructor: [`agent.NewModelAgent`](agent/model.go)
- Model interface and info: [`model.Model`](model/model.go), [`model.Info`](model/model.go)
- Flow processors and content helpers: [flow/processors.go](flow/processors.go)

---

## Adding or Changing Components

Agents
- Compose using sequential, parallel, and loop agents under [agent/](agent/).
- Keep agent-specific state local to the agent; pass through context for run-scoped data.
- Add clear tests: success path, cancellation, and error propagation.

Tools
- Implement `tool.Tool` or use function helpers in [tool/](tool/).
- Validate args strictly and fail fast with readable errors.
- Honor context cancellation and return promptly.
- Avoid goroutines that outlive the tool's context.

Models
- Add adapters under [model/](model/).
- Keep deps minimal (std http/json where possible).
- Stream when provider supports it; expose capability via [`model.Info`](model/model.go).
- Centralize request/response structs; handle API changes defensively.

Stores
- In-memory stores live under [memory/](memory/), [artifact/](artifact/), [session/](session/).
- Ensure thread safety and deterministic iteration where applicable.

---

## Testing

- Use table-driven tests for permutations.
- Race detector required for concurrency changes: `just test-race`.
- Prefer testify's `require` for preconditions and `assert` for value checks if already used in the package.
- Keep fixtures minimal and local to the package.
- Test streaming behavior: partials, final, and error paths.
- Validate determinism (e.g., sorted outputs, stable sequences).
- Examples should compile and run; keep them focused and runnable.

Coverage
- HTML report: `just cover`
- Summary by package: `just cover-summary`

---

## Style for Public APIs and Docs

- Document exported identifiers with meaningful comments.
- If you change public API behavior, update at least one of:
  - Package comment, example in [examples/](examples/), [README.md](README.md), or [TODO.md](TODO.md).
- For notable decisions, consider a short ADR-style note in docs/ (if/when added).

---

## Branches, Commits, and PRs

Branches
- Use prefixes: `feat/`, `fix/`, `refactor/`, `docs/`, `test/`.

Commits
- Conventional style preferred:
  - `feat(agent): add bounded parallel fanout`
  - `fix(artifact): copy input on save to avoid mutation`
  - `docs(readme): clarify streaming support`

PR Checklist
- [ ] `go test ./...` passes
- [ ] Race tests when relevant: `go test -race ./...`
- [ ] Lint clean: `golangci-lint run --config .golangci.yml`
- [ ] Public exports documented (where applicable)
- [ ] No stray prints or unused code
- [ ] Examples compile (if touched)
- [ ] CI is green

Workflow
- Open Draft PRs early for design feedback.
- Prefer small, focused PRs over broad refactors.
- Provide rationale and user-facing impact in the description.

---

## Releases and CI

- CI runs tests/lint and builds artifacts (see [.github/workflows/build.yml](.github/workflows/build.yml)).
- Releases are cut via GoReleaser (see [.goreleaser.yml](.goreleaser.yml)).
- Avoid unannounced breaking changes; call out deprecations clearly.

---

## Security and Secrets

- Do not commit secrets or tokens.
- Use environment variables for provider keys (e.g., `OPENAI_API_KEY`).
- Sanitize tool outputs and validate all tool inputs.

---

## Getting Help

- Open an issue or Discussion describing the problem and context.
- For larger changes, start with a Draft PR to align on design before implementation.

---

Thanks
