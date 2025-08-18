// Package core provides the foundational domain types, interfaces and execution
// contexts used by AgentMesh. It defines the core abstractions for:
//
//   - Agents (units of autonomous / orchestrated work)
//   - Sessions (stateful conversational containers with event history)
//   - Events (immutable communication + orchestration records)
//   - InvocationContext / ToolContext (scoped execution & tool sandboxing)
//   - Pluggable stores for session state, artifacts and memory recall/search
//
// The package intentionally keeps implementation concerns (persistence, engine
// orchestration, concrete agents) out of scope, exposing small interfaces to
// enable custom backends and extensions. All exported identifiers include
// concise documentation to aid discoverability and external consumption.
package core
