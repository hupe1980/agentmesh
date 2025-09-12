// Package langchaingo provides an adapter that wraps a langchaingo Tool
// (github.com/tmc/langchaingo/tools) so it can be used as an agentmesh
// core.Tool. The adapter normalizes the invocation contract to the
// agentmesh tool interface and supplies a minimal JSON Schema definition
// for argument validation.
package langchaingo
