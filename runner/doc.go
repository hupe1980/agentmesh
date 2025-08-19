// Package runner implements the core orchestration layer for AgentMesh.
//
// The Runner serves as the central coordination hub that manages the complete
// lifecycle of multi-agent conversations and workflows. It bridges the gap
// between high-level AgentMesh operations and low-level agent implementations,
// providing a robust foundation for scalable agent orchestration.
//
// NOTE: This package was formerly named "engine" and the primary struct was
// Engine. It has been renamed to runner/Runner for clarity and alignment with
// the public façade (agentmesh) which now hides most orchestration details.
//
// # Responsibilities (abridged)
//   - Agent invocation orchestration (async streaming + sync helper via façade)
//   - Event processing & side‑effect application (session state, artifacts)
//   - Session history persistence
//   - Invocation lifecycle management & cancellation
//
// See runner.go for the operational implementation details.
package runner
