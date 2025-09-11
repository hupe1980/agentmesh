// Package agent provides first-class agent implementations and helpers for
// composing reasoning/orchestration graphs in AgentMesh.
//
// It includes:
//   - BaseAgent: identity + hierarchy utilities
//   - SequentialAgent, ParallelAgent, LoopAgent: coordination patterns
//   - ModelAgent: LLM-driven conversational agent with tool calling
//
// Agents operate on core.RequestContext and stream core.Event values. Persistence,
// model- and tool-specific concerns live in their own packages to avoid cycles.
package agent
