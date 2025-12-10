// Package testutil provides testing utilities for the agentmesh framework.
//
// This package offers fluent builders, mock implementations, and assertion
// helpers designed to simplify testing of AI agents and graph-based workflows.
//
// # Overview
//
// The testutil package provides:
//   - ModelBuilder: Create mock models with configurable responses and behaviors
//   - ToolBuilder: Create mock tools with custom execution logic
//   - ConversationRecorder: Record and analyze model interactions
//   - Pre-built test scenarios for common patterns
//   - Custom assertions for agent testing
//
// # Quick Start
//
// Create a mock model that returns a simple response:
//
//	model := testutil.NewModelBuilder().
//	    WithResponse("Hello, world!").
//	    Build()
//
// Create a mock model with multiple sequential responses:
//
//	model := testutil.NewModelBuilder().
//	    WithResponses("First response", "Second response", "Third response").
//	    Build()
//
// Create a mock model with tool calls:
//
//	model := testutil.NewModelBuilder().
//	    WithToolCalls(message.ToolCall{
//	        ID:        "call_1",
//	        Name:      "search",
//	        Type:      "function",
//	        Arguments: `{"query": "test"}`,
//	    }).
//	    WithResponse("Based on the search results...").
//	    Build()
//
// Create a mock tool:
//
//	tool := testutil.NewToolBuilder("calculate").
//	    WithDescription("Performs calculations").
//	    WithResult("42").
//	    Build()
//
// Or with a custom call function:
//
//	tool := testutil.NewToolBuilder("calculate").
//	    WithCall(func(ctx context.Context, args string) (any, error) {
//	        return "42", nil
//	    }).
//	    Build()
//
// Record and analyze conversations:
//
//	recorder := testutil.NewConversationRecorder()
//	model := testutil.NewModelBuilder().
//	    WithRecorder(recorder).
//	    WithResponse("test").
//	    Build()
//
//	// After running agent...
//	recorder.AssertRequestCount(t, 1)
//	recorder.AssertContains(t, "user input")
//
// Use pre-built scenarios:
//
//	scenario := testutil.SimpleResponseScenario("Hello!")
//	// scenario.Model, scenario.Recorder are ready to use
//
// Use message assertions:
//
//	testutil.AssertMessages(t, messages,
//	    testutil.IsHuman("Hello"),
//	    testutil.IsAI(testutil.Contains("Hi")),
//	)
package testutil
