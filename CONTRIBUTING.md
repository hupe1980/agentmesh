# Contributing to agentmesh

Thanks for your interest in contributing! This guide explains how to set up a development environment, the project conventions, and the checklist to follow before submitting a pull request.

---
## 1. Quick Start
1. Fork & clone the repo
2. Install Go (>= 1.22 recommended)
3. Enable modules: `export GO111MODULE=on`
4. Run tests: `go test ./...`
5. (Optional) Race detection: `go test -race ./...`

> Tip: A Makefile with common targets is listed as a quick win in `AUDIT.md`; once added prefer `make test` / `make test-race`.

---
## 2. Project Philosophy
- Small, composable core interfaces (`core/`) - keep them minimal & stable.
- Deterministic, observable agent execution - events are the backbone.
- Clear separation between orchestration (agents/flow/engine) and integration layers (tool/, model/, artifact/, memory/, session/).
- Prefer explicit helpers over ad‑hoc reconstruction (e.g., `NewChildInvocationContext`).

See `AUDIT.md` for the recurring quality checklist & roadmap.

---
## 3. Repository Structure (Essentials)
| Dir | Purpose |
|-----|---------|
| core/ | Fundamental types: InvocationContext, Session, Events, Stores |
| agent/ | Agent implementations (sequential, parallel, loop, LLM) |
| flow/ | Flow orchestration primitives |
| tool/ | Tool interfaces & registry |
| model/ | Model client abstractions (OpenAI, Anthropic placeholder) |
| memory/, artifact/, session/ | Pluggable store implementations |
| engine/ | Execution engine / callbacks |
| examples/ | Runnable usage examples |

(Planned: `STRUCTURE.md` for deeper detail - tracked in `AUDIT.md`).

---
## 4. Development Workflow
Branch naming (suggested):
- `feat/<area>-<short-desc>`
- `fix/<area>-<issue>`
- `refactor/<area>`
- `docs/<topic>`

Open a Draft PR early for larger changes; link related audit quick win or roadmap item.

---
## 5. Coding & Style
- Run `go fmt ./...` (or rely on your editor's goimports).
- Keep exported identifiers documented (package doc + exported symbols). Avoid redundant commentary that restates code.
- Group related imports (std | third‑party | internal) if adding new ones.
- Avoid premature micro‑optimizations; justify performance changes with a brief note / benchmark.

### Errors
- Prefer sentinel errors where callers benefit (e.g., unavailable store). Pattern:
  ```go
  var ErrArtifactStoreUnavailable = errors.New("artifact: store unavailable")
  ```
- Prefix error messages with domain (`artifact:`, `memory:`, `session:`, `agent:`) for log filtering.
- Wrap underlying errors with `%w` when additional context matters.

### Concurrency
- Always respect context cancellation (`select { case <-ctx.Done(): ... }`).
- When spawning goroutines in agents, intercept & forward events carefully; use `NewChildInvocationContext` to avoid manual field drift.
- Document channel buffer sizes; prefer small, bounded buffers unless justified.

---
## 6. InvocationContext Guidelines
- Use `Clone()` for speculative / branch modifications where pending deltas should copy forward.
- Use `NewChildInvocationContext(emit, resume, branch)` when you need fresh state/artifact buffers and custom channels.
- After mutating state via `SetState`, ensure it is propagated by calling `EmitEvent` (which automatically merges & clears deltas) or `CommitStateDelta`.

---
## 7. Adding a New Agent
Checklist:
1. Define struct embedding `BaseAgent`.
2. Implement `Run(*core.InvocationContext) error` with start/stop lifecycle (`Start` / `Stop`).
3. For composite behavior requiring interception, derive contexts with `NewChildInvocationContext`.
4. Emit escalation events via `agent.CreateEscalationEvent` if escalation semantics apply.
5. Add focused tests (happy path + cancellation + error propagation + escalation if relevant).
6. Update examples if it introduces a new execution pattern.

---
## 8. Adding a Tool
1. Implement the `tool.Tool` interface.
2. Register it in `tool/registry.go` (or via dynamic registration path).
3. Provide argument validation (JSON schema or manual) - fail fast.
4. Add interface compliance assertion test.
5. Add example usage or expand an existing one.

---
## 9. Adding a Model Client
1. Create a subfolder under `model/`.
2. Implement the model interface minimally (streaming if applicable later).
3. Keep external dependencies lean; prefer standard HTTP.
4. No secret keys hardcoded - use env vars.
5. Document configuration & limits in a short README inside the subfolder.

---
## 10. Tests & Coverage
- Run: `go test ./... -count=1` (default), add `-race` for concurrency validation.
- Coverage (target evolving; see `AUDIT.md`). To generate: `go test ./... -coverprofile=coverage.out`.
- Add behavior tests for new edge cases; avoid brittle snapshot tests.
- Use interface assertions sparingly to guard stability.

---
## 11. Events & Escalation
- Use `EmitEvent` to flush pending state & artifact deltas.
- Escalation: child agent sets `Actions.Escalate = true`; parent composite agent may treat it as early success/termination.
- Keep escalation payload minimal but informative (why escalation happened).

---
## 12. Documentation Expectations
Every PR adding public API surface must include one of:
- New or updated package / type comments
- Example in `examples/` if feature-driven
- Update to `README.md` if user-facing capability

---
## 13. Commit Messages
Prefer Conventional Commit style (not enforced yet):
- `feat(agent): add retry backoff to LoopAgent`
- `fix(tool): correct nil resume channel panic`
- `docs(core): elaborate InvocationContext cloning rules`

---
## 14. Pull Request Checklist
Before marking Ready for Review:
- [ ] Tests pass locally (`go test ./...`)
- [ ] No `go vet` / (future) `golangci-lint` basic issues
- [ ] Added / updated docs for exported symbols
- [ ] Event / state handling follows InvocationContext rules
- [ ] Race-safe (run with `-race` if concurrency added)
- [ ] No unrelated reformat noise
- [ ] Updated `AUDIT.md` if resolving a quick win

---
## 15. Release Hygiene (Lightweight for now)
- Confirm no breaking API changes without version bump rationale
- Run full test suite with `-race`
- Generate & review coverage diff (trend acceptable)
- Update CHANGELOG (to be introduced) summarizing user-visible changes

---
## 16. Code of Conduct
Not yet formalized; please act professionally & respectfully. A standard Contributor Covenant can be added if community grows.

---
## 17. Questions / Help
Open a Discussion or Draft PR for architectural questions before deep implementation. Early alignment reduces churn.

---
Happy building!
