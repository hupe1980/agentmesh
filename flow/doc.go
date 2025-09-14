// Package flow orchestrates agent execution pipelines.
//
// Responsibilities:
//   - Build ModelRequest objects via request processors (instructions, memory, tools, transfers).
//   - Stream model output (partial + final) directly; each chunk is passed through registered
//     ResponseProcessors before emission.
//   - Extract and execute function/tool calls, optionally looping until no calls remain.
//   - Merge multiple function responses deterministically and propagate tool / state actions.
//   - Handle agent transfers (escalation) via FunctionResponseEvent actions to peers/parent.
//
// The flow no longer buffers model events: response processors operate per chunk to allow
// real‑time transformation or enrichment of partial outputs while preserving streaming UX.
// If post‑hoc mutation of only the final chunk is desired, a processor can simply ignore
// partial responses (resp.Partial == true) and act only on the terminal response.
//
// Selection between single‑ and multi‑agent flows is policy‑driven via a Selector.
package flow
