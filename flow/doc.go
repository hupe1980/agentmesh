// Package flow orchestrates agent execution pipelines.
//
// Responsibilities:
//   - Build ModelRequest objects via request processors (e.g. instructions, memory, tools).
//   - Invoke the shared model.ExecuteModel orchestration (emits partial + final events).
//   - Buffer emitted model events, apply response processors to the final
//     ModelResponse, then forward all events to the caller's EventWriter in order.
//   - Extract and execute function/tool calls, optionally looping until no calls remain.
//   - Handle agent transfers (escalation) via FunctionResponseEvent actions.
//
// Buffering rationale: response processors may wish to mutate or inspect the final
// ModelResponse before any assistant event becomes visible externally; buffering keeps
// the user-facing event stream stable while still allowing incremental partial output
// semantics internally.
//
// Selection between single- and multi-agent flows is policy-driven via Selector.
package flow
