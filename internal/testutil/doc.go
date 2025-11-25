// Package testutil provides reusable mock implementations and testing utilities
// for the AgentMesh framework.
//
// # Overview
//
// This package centralizes mock implementations to reduce code duplication across
// test files and improve test maintainability. All mocks are configurable via
// function fields, allowing tests to inject custom behavior as needed.
//
// # Available Mocks
//
// MockModel: Configurable mock of model.Model with customizable response generation
// MockTool: Configurable mock of tool.Tool with customizable execution behavior
// MockCheckpointer: In-memory mock of checkpoint.Checkpointer for state persistence tests
// MockNode: Configurable mock of pregel.Vertex for BSP graph testing
// MockGraph: Complete mock graph implementation for pregel runtime tests
//
// # Usage Examples
//
// MockModel with custom response:
//
//	mockModel := &testutil.MockModel{
//	    GenerateFunc: func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
//	        return func(yield func(*model.Response, error) bool) {
//	            yield(&model.Response{
//	                Message: message.NewAIMessageFromText("custom response"),
//	                Partial: false,
//	            }, nil)
//	        }
//	    },
//	}
//
// MockTool with custom execution:
//
//	mockTool := &testutil.MockTool{
//	    NameValue: "search",
//	    CallFunc: func(ctx context.Context, args string) (any, error) {
//	        return "search result: " + args, nil
//	    },
//	}
//
// MockCheckpointer with in-memory storage:
//
//	checkpointer := testutil.NewMockCheckpointer()
//	err := checkpointer.Save(ctx, checkpoint)
//	loaded, _ := checkpointer.Load(ctx, "run-123")
//
// MockGraph for pregel tests:
//
//	graph := testutil.NewMockGraph(
//	    []string{"A"},                      // root vertices
//	    map[string]pregel.Vertex[S, M]{     // vertices
//	        "A": &testutil.MockNode[S, M]{NameValue: "A", NextNode: "B"},
//	        "B": &testutil.MockNode[S, M]{NameValue: "B"},
//	    },
//	    map[string][]string{"A": {"B"}},    // edges
//	    mockState,                          // initial state
//	)
package testutil
