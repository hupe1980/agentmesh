// Package agent contains first-class agent implementations and supporting
// utilities for building composable reasoning / orchestration graphs in
// AgentMesh. The package focuses on three concerns:
//
//  1. Base lifecycle + hierarchy plumbing (BaseAgent)
//  2. Concrete coordination patterns (SequentialAgent, ParallelAgent, LoopAgent)
//  3. Model-centric conversational / tool-calling agent (ModelAgent)
//
// Design principles:
//   - Minimal hidden global state – explicit wiring via Engine/InvocationContext
//   - Composability – agents can nest arbitrarily using SetSubAgents / FindAgent
//   - Observability – clear logging hooks at start/stop and flow selection
//   - Extensibility – embed BaseAgent; only implement Run plus any custom API
//
// Execution Model:
//   - An agent's Run receives a *core.InvocationContext (shared or cloned)
//   - Composite agents (parallel / sequential / loop) coordinate child Runs
//   - ModelAgent integrates with model, tool and flow packages to stream events
//
// The package intentionally keeps persistence, model specifics and tool
// registry abstractions in their respective packages to avoid cyclic deps.
package agent
