# Contributing to agentmesh

Thanks for your interest in improving agentmesh. This guide reflects the current (Runner‑based) architecture and active conventions.

---
## 1. Quick Start
1. Fork & clone
2. Go >= 1.24
3. Run tests: `go test ./...`
4. (Optional) Race: `go test -race ./...`
5. Examples: `go run examples/basic_agent/main.go`

We use a `justfile` (add `just` locally) for future shortcuts. If a target exists prefer `just test` etc.

---
## 2. Current Philosophy
| Principle | Summary |
|-----------|---------|
| Minimal core | Keep `core/` types stable & small (Event, Content, Session, Runner interface). |
| Explicit orchestration | Agents & flows emit Events; side‑effects only through defined actions. |
| Determinism goal | Avoid nondeterministic ordering (tool calls, map iteration) for testability. |
| Streaming first | Partial events prioritized; low overhead per event. |
| Safe tools | Tool execution must be bounded (timeouts/panic recovery in progress). |

---
## 3. Repository Structure (Condensed)
| Dir | Purpose |
|-----|---------|
| core/ | Core contracts & types (Runner interface, Events, Stores) |
| runner/ | Runner implementation |
| agent/ | Agents (model, sequential, parallel, loop, etc.) |
| flow/ | Flow orchestration helpers / selectors |
| tool/ | Tool interfaces & helpers (function tools, state manager) |
| model/ | Model adapters (openai/, anthropic/ WIP) |
| memory/, artifact/, session/ | Pluggable in-memory stores (durable planned) |
| examples/ | Runnable examples |
| engine/ | Temporary deprecated alias (scheduled for removal) |
| internal/ | Test utilities & internal helpers |

---
## 4. Branch & PR Workflow
Suggested branch prefixes: `feat/`, `fix/`, `refactor/`, `docs/`, `test/`. Open Draft PRs early for non‑trivial changes. Reference internal tracking IDs when applicable.

---
## 5. Coding & Style
* `go fmt` + `go vet` clean before PR.
* Document exported identifiers meaningfully (avoid restating signature).
* Group imports: std | third‑party | internal.
* Use `context.Context` as first parameter where added.
* Prefer small focused files; avoid giant god objects.

### Errors
* Domain‑prefixed messages (`memory:`, `artifact:` ...).
* Wrap with `%w` for propagation.
* Sentinel errors (e.g. `ErrNotFound`) are being standardized; add in central location when introduced.

### Concurrency
* Always honor cancellation; short selects over blocking operations.
* Avoid unbounded goroutines for tools (semaphore coming; mimic pattern if adding interim code).
* Document channel buffer sizes.

---
## 6. Events & Invocation Context
* Use `EmitEvent` to flush state & artifact deltas.
* `Partial` = streaming fragment. Do not mutate Events after emission.
* For tool calls, create function call parts rather than piggybacking on text.
* Avoid adding random metadata; prefer structured fields or `CustomMetadata` with a short, namespaced key.


---
## 7. Adding Agents
Checklist:
1. Embed / reuse base behavior (`BaseAgent`).
2. Implement `Run(*core.InvocationContext) error` only; no other side‑channel logic.
3. Emit escalation with `Actions.Escalate` when handing off upward.
4. Add tests: success, cancellation, error propagation.
5. Update an example only if it clarifies usage (avoid churn in all examples).

---
## 8. Tools
1. Implement `tool.Tool` or use function helper.
2. Validate args strictly (schema or manual) – fail fast with readable error.
3. Anticipate forthcoming timeout wrapper; do not spawn goroutines that outlive context.
4. Provide a minimal test (success + validation failure).

---
## 9. Model Adapters
* Keep dependencies minimal (std http + json).
* Stream tokens/events incrementally; aggregate only when required.
* Protect against API changes; centralize request/response structs.
* Do not embed API keys in code; rely on env vars.

---
## 10. Tests
* Use `testify/require` for critical preconditions, `assert` for value checks (migrating legacy tests gradually).
* Run `go test ./... -race` for concurrency changes.
* Prefer table tests for permutations.
* Golden tests limited to wire format (planned for Events) – avoid overuse.

Benchmark additions should include a brief comment about scenario & target metric.

---
## 11. Documentation Expectations
Public API changes require at least one of: updated package comment, added example, or README/TODO entry. New architectural decisions: add an ADR‑style markdown (`EVENT.md` pattern) if non‑obvious.

---
## 12. Deprecations & Compatibility
* `engine/` is a temporary alias for `runner/` – do not add new code there.
* Use `Runner.Run` & `Runner.Cancel`; avoid adding new usages of deprecated `Invoke` / `CancelInvocation` shims.
* Deprecation comments must start with `// Deprecated:` and reference removal milestone.

---
## 13. Commit Messages
Conventional style preferred:
* `feat(runner): add deterministic tool ordering`
* `fix(memory): prevent nil map panic in Put`
* `docs(event): add CloudEvents rationale`

---
## 14. PR Checklist
* [ ] `go test ./...` passes
* [ ] (If concurrency) `go test -race ./...`
* [ ] Exported symbols documented
* [ ] No stray debug prints / unused code
* [ ] Events emitted via context helpers (no manual partial mutation post‑emit)
* [ ] Examples updated or consciously untouched (state decision in PR)
* [ ] Internal tracking item updated if applicable

---
## 15. Release Prep (Lightweight)
* Ensure no unannounced breaking changes
* Run full test suite with race
* Update CHANGELOG (to be added) & tag

---
## 16. Code of Conduct
Not formalized yet. Be respectful; harassment-free collaboration expected.

---
## 17. Getting Help
Open a Discussion or Draft PR for design exploration. Early feedback reduces rework.

---
Happy hacking!
