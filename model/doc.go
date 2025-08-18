// Package model defines the provider‑agnostic abstractions and concrete
// helpers for interacting with language / reasoning models inside AgentMesh.
//
// Core goals:
//   - Unify streaming + non‑streaming generation behind a single interface
//   - Normalize tool / function call representation (ToolDefinition, ToolCall)
//   - Keep request/response shapes minimal and transport independent
//   - Facilitate lightweight mocking for tests (MockModel)
//
// Providers (e.g. OpenAI, Anthropic) implement the Model interface from this
// package so higher layers (agents, flows) remain decoupled from vendor SDKs.
package model
