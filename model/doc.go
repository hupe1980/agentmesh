// Package model defines provider‑agnostic abstractions and orchestration helpers
// for interacting with language / reasoning models inside AgentMesh.
//
// Core goals:
//   - Unify streaming + non‑streaming generation behind a single interface.
//   - Normalize tool / function call representation (ToolDefinition, ToolCall).
//   - Keep request/response shapes minimal and transport independent.
//   - Centralize hook orchestration (Before / OnError / After) in one executor.
//   - Facilitate lightweight mocking for tests (MockModel / ModelExecutorMock).
//
// Execution model:
//
//	ExecuteModel(ctx, reqCtx, before, after, mdl, req) drives generation while invoking
//	RequestContext hooks in a well-defined order:
//	  1. RunBeforeModel: optional short‑circuit (returns final *ModelResponse).
//	  2. Model.Generate: stream partial + final responses.
//	  3. RunOnModelError: opportunity to recover with a replacement response.
//	  4. RunAfterModel: final post‑processing / replacement.
//	  5. Agent callbacks (before / after) run around plugin hooks when provided.
//
//	During generation each partial response produces a PartialAssistant event
//	through the provided EventWriter; the final response (original, recovered or
//	replaced) yields a FullAssistant event. The function returns the final
//	*ModelResponse for downstream processing (e.g. function call extraction).
//
// Providers (e.g. OpenAI, Anthropic) implement the Model interface so higher
// layers (agents, flows) remain decoupled from vendor SDKs.
package model
