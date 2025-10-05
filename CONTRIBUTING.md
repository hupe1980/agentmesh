# Contributing to AgentMesh

Thanks for investing time in AgentMesh! This guide helps you set up a local environment, follow the project’s conventions, and land changes smoothly without surprises.

---

## 🌟 Project values

- **Deterministic orchestration** – favour explicit control flow and predictable ordering.
- **Streaming-first UX** – emit partial results quickly and avoid unnecessary buffering.
- **Safe tooling** – validate inputs, respect context cancellation, and isolate side effects.
- **Provider agnostic** – adapters should expose capabilities consistently and degrade gracefully.
- **Testable pieces** – keep components small, focused, and covered by meaningful tests.

---

## 🧰 Local setup

Requirements:

- Go 1.24+
- `just` (optional but recommended)
- `golangci-lint` (installed via `just install-deps`)

Clone the repository and bootstrap your tools:

```sh
just install-deps
```

Everyday commands:

```sh
just            # list available tasks
just test       # go test ./...
just test-race  # go test ./... -race
just lint       # golangci-lint run
just cover      # HTML coverage report
just docs-serve # live docs preview at http://localhost:4000
```

Prefer raw tooling?

```sh
go test ./... -race
golangci-lint run --config .golangci.yml
```

---

## 🗺️ Repository tour

- [`core/`](core/) – foundational contracts (`Agent`, `Model`, events, parts, errors)
- [`agent/`](agent/) – sequential, parallel, loop, model, and functional agents
- [`runner/`](runner/) – execution orchestration, buffering, concurrency controls
- [`flow/`](flow/) – processors that plan, merge, transfer, and filter events
- [`tool/`](tool/) – function tools, agent tools, retrieval helpers, MCP adapters
- [`model/`](model/) – OpenAI, LangChainGo, gateway, and functional model adapters
- [`artifact/`](artifact/), [`session/`](session/), [`memory/`](memory/) – state stores
- [`logging/`](logging/), [`metrics/`](metrics/), [`trace/`](trace/) – observability helpers
- [`examples/`](examples/) – runnable demos showcasing tools, retrieval, tracing, transfers

Use the examples to sanity-check behavioural changes or illustrate new features.

---

## ✍️ Coding guidelines

- Format with `gofmt`; keep `go vet` and `golangci-lint` clean.
- Group imports as stdlib / third-party / internal.
- Accept `context.Context` as the first parameter for anything that can block or reach external systems.
- Keep files focused and types composable; avoid “god” structs.
- Document exported identifiers with clear, actionable comments.

### Errors

- Wrap errors using `%w` and prefix with the package domain (e.g., `retrieval:`).
- Expose sentinel errors when callers must branch on specific conditions.

### Concurrency

- Honour cancellation by checking `ctx.Err()` in long-running tasks.
- Bound goroutine fan-out with semaphores or worker pools; avoid unbounded `go func(){}` patterns.
- Document channel buffer sizes and lifetimes when they are not obvious.

### Determinism

- Preserve ordering in outputs (sort slices when required; do not rely on map iteration order).
- Ensure retrievers, mergers, and tool executors produce stable results across runs.

---

## ✅ Testing expectations

Before opening a pull request:

- `just test`
- `just lint`
- Run `just test-race` for changes that touch concurrency, flows, or state stores.
- Add or update table-driven tests covering success, failure, and cancellation paths.
- For streaming code, assert partial and final events independently.
- Keep fixtures minimal and local to the relevant package.

Coverage is not strictly enforced, but new behaviour should include unit tests when practical. Inspect coverage deltas with:

```sh
just cover
just cover-summary
```

---

## 📚 Docs & examples

- Ensure examples continue to compile (`go run ./examples/...`).
- When public behaviour changes, update at least one of:
  - `README.md`
  - Documentation under `docs/`
  - A relevant example in `examples/`
- Preview the documentation site locally via `just docs-serve` (Jekyll with live reload).

---

## 🔀 Git workflow

Branches:

- Use descriptive prefixes such as `feat/`, `fix/`, `refactor/`, `docs/`, or `test/`.

Commits:

- Prefer conventional messages, e.g. `feat(tool): add merger stop-on-first-error option`.
- Keep commits focused; split unrelated changes across separate commits.

Pull requests:

- Explain the motivation, approach, and user-facing impact.
- Link related issues or discussions.
- Ensure CI is green and tick through this checklist:
  - [ ] `go test ./...`
  - [ ] `go test -race ./...` (when concurrency is affected)
  - [ ] `golangci-lint run --config .golangci.yml`
  - [ ] Public exports documented (if applicable)
  - [ ] Examples compile when touched
  - [ ] No stray debug prints or commented code

Draft PRs are encouraged when you want early feedback before final polishing.

---

## 🚀 Releases & compatibility

- CI pipelines live in [.github/workflows/](.github/workflows/) and run lint/tests automatically.
- Releases are produced with GoReleaser ([.goreleaser.yml](.goreleaser.yml)).
- Avoid breaking changes without prior discussion; document migration steps when they are unavoidable.
- Deprecate APIs with clear comments and follow-up issues for removal.

---

## 🔐 Security & secrets

- Never commit credentials or API keys. Use environment variables during development.
- Scrub or redact sensitive information in logs and errors.
- Validate tool inputs and sanitize external outputs before surfacing them to models.

---

## 🤝 Need help?

- Open an issue describing bugs or feature requests.
- Start a Discussion to propose larger design changes or refactors.
- Mention maintainers if you need a time-sensitive review.

Thanks again for contributing and keeping AgentMesh reliable!
