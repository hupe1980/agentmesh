// Package memory contains concrete MemoryStore implementations. The store
// interface and SearchResult type reside in the core package. Import
// github.com/hupe1980/agentmesh/core and depend on core.MemoryStore in your
// code; select an implementation (like the in‑memory store below) at wiring
// time.
//
// Rationale: keeps domain contracts centralized while allowing pluggable
// backends (vector databases, embeddings indexes, etc.) to be added without
// introducing dependency cycles.
package memory
