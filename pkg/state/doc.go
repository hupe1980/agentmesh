// Package state provides core state management interfaces for AgentMesh graphs.
//
// This package defines the fundamental interfaces for reading and writing state
// in agent workflows, decoupling state access from graph execution logic.
//
// Key Interfaces:
//   - Reader: Read-only state access for node execution
//   - Writer: Read-write state access with aggregation support
//
// This separation prevents circular dependencies and allows tools, agents,
// and other packages to work with state without depending on the graph package.
package state
