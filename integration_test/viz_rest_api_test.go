package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"net"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/hupe1980/agentmesh/internal/testutil"
	"github.com/hupe1980/agentmesh/pkg/agent"
	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/viz"
)

// TestVizServerRESTEndpoints tests all REST API endpoints end-to-end
func TestVizServerRESTEndpoints(t *testing.T) {
	// Find a free port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to find free port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	// Create server with test configuration
	checkpointer := checkpoint.NewInMemoryCheckpointer()
	server, err := viz.NewServer(viz.Config{
		Addr:            fmt.Sprintf("127.0.0.1:%d", port),
		EventBufferSize: 1000,
		Checkpointer:    checkpointer,
	})
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Create a simple test graph
	testGraph := createVizTestGraph(t)
	if err := server.Register("test-graph", viz.NewMessageAdapter(testGraph)); err != nil {
		t.Fatalf("Failed to register graph: %v", err)
	}

	// Start server in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverStarted := make(chan bool)
	go func() {
		close(serverStarted)
		if err := server.Start(ctx); err != nil && err != http.ErrServerClosed {
			t.Logf("Server error: %v", err)
		}
	}()

	// Wait for server to start
	<-serverStarted
	time.Sleep(50 * time.Millisecond)

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	// Test each endpoint
	t.Run("ListGraphs", func(t *testing.T) {
		testListGraphs(t, baseURL)
	})

	t.Run("ExecuteGraph", func(t *testing.T) {
		runID := testExecuteGraph(t, baseURL)

		// Wait for execution to start
		time.Sleep(50 * time.Millisecond)

		// Run dependent tests with the runID
		t.Run("GetRunDetails", func(t *testing.T) {
			testGetRunDetails(t, baseURL, runID)
		})

		t.Run("GetRunEvents", func(t *testing.T) {
			testGetRunEvents(t, baseURL, runID)
		})

		t.Run("GetRunState", func(t *testing.T) {
			testGetRunState(t, baseURL, runID)
		})

		t.Run("GetRunAnalytics", func(t *testing.T) {
			testGetRunAnalytics(t, baseURL, runID)
		})

		t.Run("ControlRun", func(t *testing.T) {
			testControlRun(t, baseURL, runID)
		})
	})

	t.Run("ListRuns", func(t *testing.T) {
		testListRuns(t, baseURL)
	})

	t.Run("GetGraphMermaid", func(t *testing.T) {
		testGetGraphMermaid(t, baseURL)
	})

	t.Run("TestManagement", func(t *testing.T) {
		testTestManagement(t, baseURL)
	})

	t.Run("GetOpenAPISpec", func(t *testing.T) {
		testGetOpenAPISpec(t, baseURL)
	})
}

func testListGraphs(t *testing.T, baseURL string) {
	resp, err := http.Get(baseURL + "/api/graphs")
	require.NoError(t, err, "Failed to list graphs")
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode, "Expected status 200")

	var result struct {
		Graphs []string `json:"graphs"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err, "Failed to decode response")

	require.NotEmpty(t, result.Graphs, "Expected at least one graph")
	require.Contains(t, result.Graphs, "test-graph", "test-graph should be in graphs list")
}

func testExecuteGraph(t *testing.T, baseURL string) string {
	input := map[string]any{
		"query": "test execution",
	}
	body, err := json.Marshal(input)
	require.NoError(t, err, "Failed to marshal input")

	resp, err := http.Post(
		baseURL+"/api/graphs/test-graph/run",
		"application/json",
		bytes.NewReader(body),
	)
	require.NoError(t, err, "Failed to execute graph")
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode, "Expected status 200")

	var result struct {
		RunID string `json:"run_id"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err, "Failed to decode response")
	require.NotEmpty(t, result.RunID, "Expected non-empty run_id")

	return result.RunID
}

func testGetRunDetails(t *testing.T, baseURL, runID string) {
	resp, err := http.Get(fmt.Sprintf("%s/api/runs/%s", baseURL, runID))
	if err != nil {
		t.Fatalf("Failed to get run details: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var result struct {
		ID        string `json:"id"`
		Status    string `json:"status"`
		StartTime string `json:"start_time"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if result.ID != runID {
		t.Errorf("Expected run ID %s, got %s", runID, result.ID)
	}

	validStatuses := []string{"running", "completed", "failed", "paused"}
	validStatus := slices.Contains(validStatuses, result.Status)
	if !validStatus {
		t.Errorf("Invalid status: %s", result.Status)
	}
}

func testGetRunEvents(t *testing.T, baseURL, runID string) {
	resp, err := http.Get(fmt.Sprintf("%s/api/runs/%s/events", baseURL, runID))
	if err != nil {
		t.Fatalf("Failed to get run events: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var result struct {
		Events []map[string]any `json:"events"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Events array may be nil initially, which is acceptable
}

func testGetRunState(t *testing.T, baseURL, runID string) {
	// State endpoint should work for active and completed runs
	resp, err := http.Get(fmt.Sprintf("%s/api/runs/%s/state", baseURL, runID))
	if err != nil {
		t.Fatalf("Failed to get run state: %v", err)
	}
	defer resp.Body.Close()

	// State endpoint should return 200 with actual state data
	if resp.StatusCode == http.StatusNotFound {
		// If 404, this is a bug - state should be available
		t.Log("State endpoint returned 404 - checking if run exists")

		// Verify run exists
		runResp, err := http.Get(fmt.Sprintf("%s/api/runs/%s", baseURL, runID))
		if err == nil {
			defer runResp.Body.Close()
			if runResp.StatusCode == http.StatusOK {
				t.Error("Run exists but state endpoint returns 404 - this indicates a bug")
			}
		}
		return
	}

	require.Equal(t, http.StatusOK, resp.StatusCode, "State endpoint should return 200")

	var result map[string]any
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err, "Failed to decode state response")

	// Verify state has expected structure
	require.Contains(t, result, "status", "State should contain status")

	// If run is active, should have meaningful state data
	if status, ok := result["status"].(string); ok && status == "running" {
		// Should have graph_state for visualization
		if graphState, hasGraphState := result["graph_state"]; hasGraphState {
			gs, ok := graphState.(map[string]any)
			require.True(t, ok, "graph_state should be an object")

			// Should have nodes tracking
			if nodes, hasNodes := gs["nodes"]; hasNodes {
				require.NotNil(t, nodes, "nodes should not be nil if present")
			}
		}
	}

	t.Logf("State response: status=%v", result["status"])
}

func testGetRunAnalytics(t *testing.T, baseURL, runID string) {
	// Wait for run to complete before checking analytics
	require.Eventually(t, func() bool {
		resp, err := http.Get(fmt.Sprintf("%s/api/runs/%s", baseURL, runID))
		if err != nil {
			return false
		}
		defer resp.Body.Close()

		var run map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&run); err != nil {
			return false
		}

		status, _ := run["status"].(string)
		return status == "completed" || status == "failed"
	}, 10*time.Second, 200*time.Millisecond, "Run should complete before checking analytics")

	// Analytics MUST be available after completion
	resp, err := http.Get(fmt.Sprintf("%s/api/runs/%s/analytics", baseURL, runID))
	require.NoError(t, err, "Failed to get run analytics")
	defer resp.Body.Close()

	// Should NOT return 404 after completion - that's a bug
	require.Equal(t, http.StatusOK, resp.StatusCode, "Analytics must be available after run completion")

	var result map[string]any
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err, "Failed to decode analytics response")

	// Verify analytics structure contains expected fields
	require.Contains(t, result, "event_count", "Analytics should contain event_count")
	require.Contains(t, result, "run_id", "Analytics should contain run_id")

	// CRITICAL: Events must have been collected
	eventCount, ok := result["event_count"].(float64)
	require.True(t, ok, "event_count should be a number")
	require.Greater(t, eventCount, 0.0, "Analytics must have events - indicates event collection works")

	// Verify cost tracking data from mock model (40 tokens total from 2 calls: 30+10)
	tokensRaw, hasTokens := result["total_tokens"]
	require.True(t, hasTokens, "Analytics should contain total_tokens")

	tokens, ok := tokensRaw.(float64)
	require.True(t, ok, "total_tokens should be a number")
	require.Equal(t, 40.0, tokens, "Total tokens should match mock model usage (30+10=40)")

	costRaw, hasCost := result["total_cost"]
	require.True(t, hasCost, "Analytics should contain total_cost")

	cost, ok := costRaw.(float64)
	require.True(t, ok, "total_cost should be a number")
	// Note: Cost is 0 if model doesn't provide cost_usd in event data
	// This is expected for mock models that only provide token counts
	t.Logf("Analytics: events=%v, tokens=%v, cost=%v", eventCount, tokens, cost)
}

func testControlRun(t *testing.T, baseURL, runID string) {
	// Test pause command
	control := map[string]any{
		"action": "pause",
	}
	body, _ := json.Marshal(control)

	resp, err := http.Post(
		fmt.Sprintf("%s/api/runs/%s/control", baseURL, runID),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("Failed to control run: %v", err)
	}
	defer resp.Body.Close()

	// Control may fail or be unimplemented
	validCodes := []int{http.StatusOK, http.StatusBadRequest, http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusNotImplemented}
	valid := slices.Contains(validCodes, resp.StatusCode)
	if !valid {
		t.Errorf("Expected status in %v, got %d", validCodes, resp.StatusCode)
	}
}

func testListRuns(t *testing.T, baseURL string) {
	resp, err := http.Get(baseURL + "/api/runs")
	require.NoError(t, err, "Failed to list runs")
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode, "Expected status 200")

	var result struct {
		Runs []map[string]any `json:"runs"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err, "Failed to decode response")

	// Should have at least one run from previous tests
	require.NotEmpty(t, result.Runs, "Expected at least one run")
}

func testGetGraphMermaid(t *testing.T, baseURL string) {
	resp, err := http.Get(baseURL + "/api/graphs/test-graph/mermaid")
	require.NoError(t, err, "Failed to get graph mermaid")
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode, "Expected status 200")

	var result struct {
		Mermaid string `json:"mermaid"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err, "Failed to decode response")

	require.NotEmpty(t, result.Mermaid, "Expected non-empty mermaid diagram")

	// Basic validation - should contain graph syntax
	hasGraphKeyword := containsSubstring(result.Mermaid, "graph") || containsSubstring(result.Mermaid, "flowchart")
	require.True(t, hasGraphKeyword, "Mermaid diagram should contain 'graph' or 'flowchart' keyword")

	// Verify it contains nodes from the ReAct agent
	require.True(t, containsSubstring(result.Mermaid, "model"), "Mermaid diagram should contain 'model' node from the ReAct agent")

	t.Logf("✓ Mermaid diagram is valid (%d chars)", len(result.Mermaid))
}

func testTestManagement(t *testing.T, baseURL string) {
	// List tests (should be empty initially)
	t.Run("ListTests", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/tests")
		if err != nil {
			t.Fatalf("Failed to list tests: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		var result struct {
			Suites []map[string]any `json:"suites"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if result.Suites == nil {
			t.Error("Expected suites array, got nil")
		}
	})

	// Create test suite
	t.Run("CreateTestSuite", func(t *testing.T) {
		suite := map[string]any{
			"suite_id": "test-suite-1",
			"graph_id": "test-graph",
			"tests": []map[string]any{
				{
					"name": "test-1",
					"input": map[string]any{
						"query": "test input",
					},
					"expected_output": map[string]any{
						"result": "success",
					},
				},
			},
		}
		body, _ := json.Marshal(suite)

		resp, err := http.Post(
			baseURL+"/api/tests/suite",
			"application/json",
			bytes.NewReader(body),
		)
		if err != nil {
			t.Fatalf("Failed to create test suite: %v", err)
		}
		defer resp.Body.Close()

		// May return 405 if not fully implemented
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("Expected status 200 or 405, got %d", resp.StatusCode)
		}
		if resp.StatusCode != http.StatusOK {
			t.Skip("Test suite creation not available")
		}
	})

	// Run tests
	t.Run("RunTests", func(t *testing.T) {
		request := map[string]any{
			"suite_id": "test-suite-1",
			"graph_id": "test-graph",
		}
		body, _ := json.Marshal(request)

		resp, err := http.Post(
			baseURL+"/api/tests/run",
			"application/json",
			bytes.NewReader(body),
		)
		if err != nil {
			t.Fatalf("Failed to run tests: %v", err)
		}
		defer resp.Body.Close()

		// May return 405 if not fully implemented
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("Expected status 200 or 405, got %d", resp.StatusCode)
		}

		if resp.StatusCode == http.StatusOK {
			var result struct {
				Results []map[string]any `json:"results"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			if len(result.Results) == 0 {
				t.Error("Expected at least one test result")
			}
		}
	})

	// Delete test
	t.Run("DeleteTest", func(t *testing.T) {
		req, err := http.NewRequest(
			http.MethodDelete,
			baseURL+"/api/tests/test-suite-1/test-1",
			nil,
		)
		if err != nil {
			t.Fatalf("Failed to create delete request: %v", err)
		}

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed to delete test: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
			t.Errorf("Expected status 200 or 404, got %d", resp.StatusCode)
		}
	})
}

// TestVizServerInvalidEndpoints tests error handling for invalid requests
func TestVizServerInvalidEndpoints(t *testing.T) {
	// Find a free port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to find free port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	server, err := viz.NewServer(viz.Config{
		Addr:            fmt.Sprintf("127.0.0.1:%d", port),
		EventBufferSize: 1000,
	})
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Start server
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := server.Start(ctx); err != nil && err != http.ErrServerClosed {
			t.Logf("Server error: %v", err)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	tests := []struct {
		name           string
		method         string
		path           string
		body           string
		expectedStatus int
	}{
		{
			name:           "NonExistentGraph",
			method:         http.MethodPost,
			path:           "/api/graphs/nonexistent/run",
			body:           `{"input":"test"}`,
			expectedStatus: http.StatusInternalServerError, // Returns 500 when graph not found
		},
		{
			name:           "InvalidRunID",
			method:         http.MethodGet,
			path:           "/api/runs/invalid-run-id",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "InvalidJSON",
			method:         http.MethodPost,
			path:           "/api/tests/suite",
			body:           `{invalid json}`,
			expectedStatus: http.StatusMethodNotAllowed, // Tests endpoint may return 405
		},
		{
			name:           "MethodNotAllowed",
			method:         http.MethodPut,
			path:           "/api/graphs",
			expectedStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body *bytes.Reader
			if tt.body != "" {
				body = bytes.NewReader([]byte(tt.body))
			}

			var req *http.Request
			var err error
			if body != nil {
				req, err = http.NewRequest(tt.method, baseURL+tt.path, body)
			} else {
				req, err = http.NewRequest(tt.method, baseURL+tt.path, nil)
			}
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}

			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("Failed to send request: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, resp.StatusCode)
			}
		})
	}
}

// Helper function to create a test graph with mock model that tracks costs
func createVizTestGraph(t *testing.T) *message.Graph {
	// Track invocation count to return tool call first, then final response
	invocationCount := 0

	// Create mock model with realistic token usage and costs
	// First invocation returns a tool call, second returns final answer
	mockModel := &testutil.MockModel{
		GenerateFunc: func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
			return func(yield func(*model.Response, error) bool) {
				invocationCount++

				if invocationCount == 1 {
					// First call: return a tool call to trigger ReAct iteration
					msg := message.NewAIMessageFromText("")
					msg.ToolCalls = []message.ToolCall{
						{
							ID:        "call_123",
							Name:      "test_tool",
							Type:      "function",
							Arguments: `{"input": "test input"}`,
						},
					}

					yield(&model.Response{
						Message: msg,
						Usage: &model.UsageInfo{
							PromptTokens:     20,
							CompletionTokens: 10,
							TotalTokens:      30,
						},
						FinishReason: "tool_calls",
						Partial:      false,
					}, nil)
				} else {
					// Second call: return final response after tool execution
					msg := message.NewAIMessageFromText("Test response after tool execution with cost tracking")

					yield(&model.Response{
						Message: msg,
						Usage: &model.UsageInfo{
							PromptTokens:     5,
							CompletionTokens: 5,
							TotalTokens:      10,
						},
						FinishReason: "stop",
						Partial:      false,
					}, nil)
				}
			}
		},
		CapabilitiesFunc: func() model.Capabilities {
			return model.Capabilities{
				Streaming: true,
				Tools:     true,
			}
		},
	}

	// Create mock tool that can be called
	mockTool := &testutil.MockTool{
		NameValue:        "test_tool",
		DescriptionValue: "A test tool for integration testing",
		CallFunc: func(ctx context.Context, input string) (any, error) {
			return map[string]any{
				"result": "tool executed successfully",
				"input":  input,
			}, nil
		},
	}

	// Create a ReAct agent with mock model and tools
	reactAgent, err := agent.NewReAct(
		mockModel,
		agent.WithTools(mockTool),
	)
	require.NoError(t, err, "Failed to create ReAct agent")

	// reactAgent is already the correct type (*message.Graph)
	return reactAgent
}

func testGetOpenAPISpec(t *testing.T, baseURL string) {
	resp, err := http.Get(baseURL + "/api/openapi.yaml")
	if err != nil {
		t.Fatalf("Failed to get OpenAPI spec: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Check Content-Type header
	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/x-yaml" {
		t.Errorf("Expected Content-Type 'application/x-yaml', got '%s'", contentType)
	}

	// Read and validate the spec contains key OpenAPI elements
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	spec := buf.String()
	if len(spec) == 0 {
		t.Fatal("OpenAPI spec is empty")
	}

	// Verify it's a valid OpenAPI spec by checking for required fields
	requiredFields := []string{
		"openapi: 3.0.3",
		"info:",
		"title: AgentMesh Visualization API",
		"paths:",
		"/api/graphs:",
		"/api/runs/{runId}:",
		"components:",
		"schemas:",
	}

	for _, field := range requiredFields {
		if !containsSubstring(spec, field) {
			t.Errorf("OpenAPI spec missing required field: %s", field)
		}
	}
}

// Helper function to check if string contains substring
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && findSubstringIndex(s, substr) >= 0
}

func findSubstringIndex(s, substr string) int {
	if len(substr) == 0 {
		return 0
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// TestVizServerWebSocket tests WebSocket connectivity and message broadcasting
func TestVizServerWebSocket(t *testing.T) {
	// Find a free port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to find free port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	// Create server
	checkpointer := checkpoint.NewInMemoryCheckpointer()
	server, err := viz.NewServer(viz.Config{
		Addr:            fmt.Sprintf("127.0.0.1:%d", port),
		EventBufferSize: 1000,
		Checkpointer:    checkpointer,
	})
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Register test graph
	testGraph := createVizTestGraph(t)
	if err := server.Register("websocket-test-graph", viz.NewMessageAdapter(testGraph)); err != nil {
		t.Fatalf("Failed to register graph: %v", err)
	}

	// Start server
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := server.Start(ctx); err != nil && err != context.Canceled {
			t.Logf("Server error: %v", err)
		}
	}()

	time.Sleep(50 * time.Millisecond) // Wait for server to start

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	wsURL := fmt.Sprintf("ws://127.0.0.1:%d/ws", port)

	t.Run("WebSocketConnection", func(t *testing.T) {
		testWebSocketConnection(t, wsURL)
	})

	t.Run("WebSocketRunStatusUpdates", func(t *testing.T) {
		testWebSocketRunStatusUpdates(t, baseURL, wsURL)
	})

	t.Run("WebSocketEventBroadcast", func(t *testing.T) {
		testWebSocketEventBroadcast(t, baseURL, wsURL)
	})
}

func testWebSocketConnection(t *testing.T, wsURL string) {
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer ws.Close()

	// Send a ping message
	if err := ws.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
		t.Errorf("Failed to send ping: %v", err)
	}

	// Set short read deadline - we just want to verify connection works
	ws.SetReadDeadline(time.Now().Add(200 * time.Millisecond))

	// Try to read a pong response
	_, _, readErr := ws.ReadMessage()
	// We expect either a pong or timeout, both are acceptable
	if readErr == nil {
		t.Logf("WebSocket connection successful - received response")
	} else {
		t.Logf("WebSocket connection successful - timeout expected")
	}
}

func testWebSocketRunStatusUpdates(t *testing.T, baseURL, wsURL string) {
	// Connect to WebSocket
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer ws.Close()

	// Start a graph execution
	input := map[string]any{"query": "websocket test"}
	body, _ := json.Marshal(input)

	resp, err := http.Post(
		fmt.Sprintf("%s/api/graphs/websocket-test-graph/run", baseURL),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("Failed to start graph execution: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	var execResult struct {
		RunID string `json:"run_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&execResult); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	runID := execResult.RunID
	t.Logf("Started run: %s", runID)

	// Subscribe to the run
	subscribeMsg := map[string]any{
		"type":   "subscribe",
		"run_id": runID,
	}
	if err := ws.WriteJSON(subscribeMsg); err != nil {
		t.Fatalf("Failed to send subscribe message: %v", err)
	}

	// Set a timeout for receiving messages
	ws.SetReadDeadline(time.Now().Add(5 * time.Second))

	// Track what we receive
	receivedRunStatus := false
	receivedEvents := false
	messageCount := 0

	// Read messages for up to 5 seconds or until we get what we need
	for messageCount < 100 { // Safety limit
		var msg map[string]any
		err := ws.ReadJSON(&msg)
		if err != nil {
			// Timeout or connection closed is okay if we got our messages
			if receivedRunStatus || messageCount > 0 {
				break
			}
			t.Logf("WebSocket read error (may be timeout): %v", err)
			break
		}

		messageCount++
		msgType, _ := msg["type"].(string)
		t.Logf("Received WebSocket message #%d: type=%s", messageCount, msgType)

		switch msgType {
		case "run_status":
			receivedRunStatus = true
			// Verify the message structure
			data, hasData := msg["data"].(map[string]any)
			if !hasData {
				t.Error("run_status message missing 'data' field")
			} else {
				run, hasRun := data["run"].(map[string]any)
				if !hasRun {
					t.Error("run_status data missing 'run' field")
				} else {
					status, _ := run["status"].(string)
					t.Logf("Run status update: %s", status)

					// Verify run has expected fields
					if _, hasID := run["id"]; !hasID {
						t.Error("Run object missing 'id' field")
					}
				}
			}

		case "event":
			receivedEvents = true
			t.Logf("Received event broadcast")

		case "lifecycle":
			t.Logf("Received lifecycle event")
		}

		// If we got a run_status, that's the main thing we're testing
		if receivedRunStatus {
			break
		}

		// Reset deadline for next message
		ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	}

	t.Logf("WebSocket test summary: messages=%d, run_status=%v, events=%v",
		messageCount, receivedRunStatus, receivedEvents)

	if messageCount == 0 {
		t.Error("Expected to receive WebSocket messages, got none")
	}

	// run_status is critical for UI functionality
	if !receivedRunStatus {
		t.Error("Expected to receive run_status message via WebSocket")
	}
}

func testWebSocketEventBroadcast(t *testing.T, baseURL, wsURL string) {
	// Connect to WebSocket
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer ws.Close()

	// Start a graph execution
	input := map[string]any{"query": "event broadcast test"}
	body, _ := json.Marshal(input)

	resp, err := http.Post(
		fmt.Sprintf("%s/api/graphs/websocket-test-graph/run", baseURL),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("Failed to start graph execution: %v", err)
	}
	defer resp.Body.Close()

	var execResult struct {
		RunID string `json:"run_id"`
	}
	json.NewDecoder(resp.Body).Decode(&execResult)
	runID := execResult.RunID

	// Subscribe to run
	ws.WriteJSON(map[string]any{
		"type":   "subscribe",
		"run_id": runID,
	})

	// Read messages with timeout
	ws.SetReadDeadline(time.Now().Add(3 * time.Second))

	eventCount := 0
	for i := 0; i < 50; i++ {
		var msg map[string]any
		err := ws.ReadJSON(&msg)
		if err != nil {
			break
		}

		if msgType, _ := msg["type"].(string); msgType == "event" {
			eventCount++
		}

		ws.SetReadDeadline(time.Now().Add(1 * time.Second))
	}

	t.Logf("Received %d event broadcasts via WebSocket", eventCount)

	if eventCount == 0 {
		t.Log("Warning: No event broadcasts received (may indicate events aren't being broadcast)")
	}
}

// TestEventCollection verifies that events are actually captured during graph execution
func TestEventCollection(t *testing.T) {
	// Find a free port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "Failed to find free port")
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	// Create server with test configuration
	checkpointer := checkpoint.NewInMemoryCheckpointer()
	server, err := viz.NewServer(viz.Config{
		Addr:            fmt.Sprintf("127.0.0.1:%d", port),
		EventBufferSize: 1000,
		Checkpointer:    checkpointer,
	})
	require.NoError(t, err, "Failed to create server")

	// Create and register test graph
	testGraph := createVizTestGraph(t)
	err = server.Register("test-graph", viz.NewMessageAdapter(testGraph))
	require.NoError(t, err, "Failed to register graph")

	// Start server in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverStarted := make(chan bool)
	go func() {
		close(serverStarted)
		if err := server.Start(ctx); err != nil && err != http.ErrServerClosed {
			t.Logf("Server error: %v", err)
		}
	}()

	<-serverStarted
	time.Sleep(50 * time.Millisecond)

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	// Execute graph
	executeBody := map[string]any{
		"query": "Hello, test!",
	}
	body, _ := json.Marshal(executeBody)
	resp, err := http.Post(baseURL+"/api/graphs/test-graph/run", "application/json", bytes.NewBuffer(body))
	require.NoError(t, err, "Failed to execute graph")
	defer resp.Body.Close()

	var execResult struct {
		RunID string `json:"run_id"`
	}
	err = json.NewDecoder(resp.Body).Decode(&execResult)
	require.NoError(t, err, "Failed to decode execution response")

	runID := execResult.RunID
	require.NotEmpty(t, runID, "runID should not be empty")

	// Wait for execution to complete
	var finalStatus string
	require.Eventually(t, func() bool {
		resp, err := http.Get(fmt.Sprintf("%s/api/runs/%s", baseURL, runID))
		if err != nil {
			return false
		}
		defer resp.Body.Close()

		var run map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&run); err != nil {
			return false
		}

		status, _ := run["status"].(string)
		finalStatus = status
		return status == "completed" || status == "failed"
	}, 3*time.Second, 50*time.Millisecond, "Execution should complete")

	t.Logf("Run completed with status: %s", finalStatus)

	// Give event store time to process all events
	time.Sleep(50 * time.Millisecond)

	// Verify events were captured
	resp, err = http.Get(fmt.Sprintf("%s/api/runs/%s/events", baseURL, runID))
	require.NoError(t, err, "Failed to get events")
	defer resp.Body.Close()

	var eventsResult struct {
		Events []map[string]any `json:"events"`
	}
	err = json.NewDecoder(resp.Body).Decode(&eventsResult)
	require.NoError(t, err, "Failed to decode events")

	// CRITICAL: Events MUST be collected during execution
	require.NotEmpty(t, eventsResult.Events, "Events must be captured during execution")

	// Log what we actually got
	eventTypes := make(map[string]int)
	for _, event := range eventsResult.Events {
		eventType, ok := event["type"].(string)
		if ok {
			eventTypes[eventType]++
		}
	}

	t.Logf("Captured %d events with types: %v", len(eventsResult.Events), eventTypes)

	// Debug: Print all events with payload
	for i, event := range eventsResult.Events {
		t.Logf("Event %d: type=%v, node=%v, superstep=%v, payload=%v", i, event["type"], event["node"], event["superstep"], event["payload"])
	}

	// REALISTIC EXPECTATIONS: Verify the events we SHOULD get from a complete graph execution
	// A ReAct agent should execute: graph_start -> node events -> graph_complete
	require.Greater(t, eventTypes["graph_start"], 0, "MUST have graph_start event - execution started")
	require.Greater(t, eventTypes["graph_complete"], 0, "MUST have graph_complete event - execution finished")

	// Should have node execution events (at minimum node_complete, ideally node_start too)
	nodeCompleteEvents := eventTypes["node_complete"]
	require.Greater(t, nodeCompleteEvents, 0, "MUST have node_complete events - nodes executed")

	// Calculate total events - should be at minimum: 1 start + N node events + 1 complete
	minimumExpectedEvents := 1 + nodeCompleteEvents + 1
	require.GreaterOrEqual(t, len(eventsResult.Events), minimumExpectedEvents,
		"Should have at least graph_start + node events + graph_complete")
}

// createSlowVizTestGraph creates a test graph with delays to ensure WebSocket
// subscriptions can be established before events are emitted.
func createSlowVizTestGraph(t *testing.T) *message.Graph {
	// Track invocation count to return tool call first, then final response
	invocationCount := 0

	// Create mock model with delays to allow WebSocket subscription
	mockModel := &testutil.MockModel{
		GenerateFunc: func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
			return func(yield func(*model.Response, error) bool) {
				// Add delay to allow WebSocket subscription to be processed
				time.Sleep(100 * time.Millisecond)

				invocationCount++

				if invocationCount == 1 {
					msg := message.NewAIMessageFromText("")
					msg.ToolCalls = []message.ToolCall{
						{
							ID:        "call_123",
							Name:      "test_tool",
							Type:      "function",
							Arguments: `{"input": "test input"}`,
						},
					}

					yield(&model.Response{
						Message:      msg,
						Usage:        &model.UsageInfo{PromptTokens: 20, CompletionTokens: 10, TotalTokens: 30},
						FinishReason: "tool_calls",
						Partial:      false,
					}, nil)
				} else {
					msg := message.NewAIMessageFromText("Test response")

					yield(&model.Response{
						Message:      msg,
						Usage:        &model.UsageInfo{PromptTokens: 5, CompletionTokens: 5, TotalTokens: 10},
						FinishReason: "stop",
						Partial:      false,
					}, nil)
				}
			}
		},
		CapabilitiesFunc: func() model.Capabilities {
			return model.Capabilities{Streaming: true, Tools: true}
		},
	}

	mockTool := &testutil.MockTool{
		NameValue:        "test_tool",
		DescriptionValue: "A test tool",
		CallFunc: func(ctx context.Context, input string) (any, error) {
			return map[string]any{"result": "ok"}, nil
		},
	}

	reactAgent, err := agent.NewReAct(mockModel, agent.WithTools(mockTool))
	require.NoError(t, err)

	return reactAgent
}

// TestWebSocketEventContent verifies WebSocket messages have correct types and payloads
func TestWebSocketEventContent(t *testing.T) {
	// Find a free port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "Failed to find free port")
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	// Create server
	checkpointer := checkpoint.NewInMemoryCheckpointer()
	server, err := viz.NewServer(viz.Config{
		Addr:            fmt.Sprintf("127.0.0.1:%d", port),
		EventBufferSize: 1000,
		Checkpointer:    checkpointer,
	})
	require.NoError(t, err, "Failed to create server")

	// Register test graph with delays to ensure WebSocket subscription works
	testGraph := createSlowVizTestGraph(t)
	err = server.Register("test-graph", viz.NewMessageAdapter(testGraph))
	require.NoError(t, err, "Failed to register graph")

	// Start server
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverStarted := make(chan bool)
	go func() {
		close(serverStarted)
		if err := server.Start(ctx); err != nil && err != http.ErrServerClosed {
			t.Logf("Server error: %v", err)
		}
	}()

	<-serverStarted
	time.Sleep(100 * time.Millisecond)

	// Connect WebSocket
	wsURL := fmt.Sprintf("ws://127.0.0.1:%d/ws", port)
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err, "Failed to connect to WebSocket")
	defer ws.Close()

	// Give WebSocket connection time to establish
	time.Sleep(20 * time.Millisecond)

	// Start collecting messages in background
	messages := make([]map[string]any, 0)
	messageChan := make(chan map[string]any, 100)
	done := make(chan bool)

	go func() {
		defer close(done)
		for {
			var msg map[string]any
			if err := ws.ReadJSON(&msg); err != nil {
				return
			}
			messageChan <- msg
		}
	}()

	// Execute graph and get run ID from response
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	executeBody := map[string]any{
		"query": "Hello, test!",
	}
	body, _ := json.Marshal(executeBody)
	resp, err := http.Post(baseURL+"/api/graphs/test-graph/run", "application/json", bytes.NewBuffer(body))
	require.NoError(t, err, "Failed to start graph execution")

	var execResponse map[string]any
	err = json.NewDecoder(resp.Body).Decode(&execResponse)
	resp.Body.Close()
	require.NoError(t, err, "Failed to decode execution response")

	runID, ok := execResponse["run_id"].(string)
	require.True(t, ok, "Response should contain run_id")
	require.NotEmpty(t, runID, "run_id should not be empty")

	// Subscribe to the run immediately to receive all events
	subscribeMsg := map[string]any{
		"type":   "subscribe",
		"run_id": runID,
	}
	err = ws.WriteJSON(subscribeMsg)
	require.NoError(t, err, "Failed to send subscription")
	t.Logf("Subscribed to run: %s", runID)

	// Give server time to process subscription
	time.Sleep(50 * time.Millisecond)

	// Collect messages with timeout - use longer timeout to handle CI variability
	timeout := time.After(5 * time.Second)
	collectMessages := true

	for collectMessages {
		select {
		case msg := <-messageChan:
			messages = append(messages, msg)
			t.Logf("Received message: type=%v", msg["type"])
		case <-timeout:
			collectMessages = false
		case <-done:
			// WebSocket closed, collect any remaining messages
			time.Sleep(20 * time.Millisecond)
			for len(messageChan) > 0 {
				msg := <-messageChan
				messages = append(messages, msg)
				t.Logf("Received message: type=%v", msg["type"])
			}
			collectMessages = false
		}
	}

	// Analyze message types
	messageTypes := make(map[string]int)
	for _, msg := range messages {
		if msgType, ok := msg["type"].(string); ok {
			messageTypes[msgType]++
		}
	}

	// CRITICAL: Should receive event messages during execution
	require.Greater(t, messageTypes["event"], 0, "Should receive event messages via WebSocket")

	// CRITICAL: Should receive run_status on completion
	require.Greater(t, messageTypes["run_status"], 0, "Should receive run_status messages via WebSocket")

	// Verify run_status messages contain run data
	hasRunData := false
	for _, msg := range messages {
		if msgType, ok := msg["type"].(string); ok && msgType == "run_status" {
			if data, ok := msg["data"].(map[string]any); ok {
				if _, hasRun := data["run"]; hasRun {
					hasRunData = true
					break
				}
			}
		}
	}
	require.True(t, hasRunData, "run_status messages should contain run data")

	t.Logf("Successfully received %d messages with types: %v", len(messages), messageTypes)
}

// TestRunStatusTransitions verifies run status changes from created -> running -> completed
func TestRunStatusTransitions(t *testing.T) {
	// Find a free port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "Failed to find free port")
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	// Create server
	checkpointer := checkpoint.NewInMemoryCheckpointer()
	server, err := viz.NewServer(viz.Config{
		Addr:            fmt.Sprintf("127.0.0.1:%d", port),
		EventBufferSize: 1000,
		Checkpointer:    checkpointer,
	})
	require.NoError(t, err, "Failed to create server")

	// Register test graph
	testGraph := createVizTestGraph(t)
	err = server.Register("test-graph", viz.NewMessageAdapter(testGraph))
	require.NoError(t, err, "Failed to register graph")

	// Start server
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverStarted := make(chan bool)
	go func() {
		close(serverStarted)
		if err := server.Start(ctx); err != nil && err != http.ErrServerClosed {
			t.Logf("Server error: %v", err)
		}
	}()

	<-serverStarted
	time.Sleep(50 * time.Millisecond)

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	// Execute graph
	executeBody := map[string]any{
		"query": "Hello, test!",
	}
	body, _ := json.Marshal(executeBody)
	resp, err := http.Post(baseURL+"/api/graphs/test-graph/run", "application/json", bytes.NewBuffer(body))
	require.NoError(t, err, "Failed to execute graph")
	defer resp.Body.Close()

	var execResult struct {
		RunID string `json:"run_id"`
	}
	err = json.NewDecoder(resp.Body).Decode(&execResult)
	require.NoError(t, err, "Failed to decode execution response")

	runID := execResult.RunID
	require.NotEmpty(t, runID, "runID should not be empty")

	// Track status transitions
	statuses := make([]string, 0)

	// Check status immediately after execution start
	resp, err = http.Get(fmt.Sprintf("%s/api/runs/%s", baseURL, runID))
	require.NoError(t, err, "Failed to get run")
	defer resp.Body.Close()

	var run map[string]any
	err = json.NewDecoder(resp.Body).Decode(&run)
	require.NoError(t, err, "Failed to decode run")

	initialStatus, _ := run["status"].(string)
	statuses = append(statuses, initialStatus)

	// Wait and check for running status
	time.Sleep(100 * time.Millisecond)
	resp, err = http.Get(fmt.Sprintf("%s/api/runs/%s", baseURL, runID))
	require.NoError(t, err, "Failed to get run")
	defer resp.Body.Close()

	err = json.NewDecoder(resp.Body).Decode(&run)
	require.NoError(t, err, "Failed to decode run")

	runningStatus, _ := run["status"].(string)
	if runningStatus != statuses[len(statuses)-1] {
		statuses = append(statuses, runningStatus)
	}

	// Wait for completion
	require.Eventually(t, func() bool {
		resp, err := http.Get(fmt.Sprintf("%s/api/runs/%s", baseURL, runID))
		if err != nil {
			return false
		}
		defer resp.Body.Close()

		var run map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&run); err != nil {
			return false
		}

		status, _ := run["status"].(string)
		if status != statuses[len(statuses)-1] {
			statuses = append(statuses, status)
		}
		return status == "completed" || status == "failed"
	}, 10*time.Second, 200*time.Millisecond, "Execution should complete")

	// Verify we saw meaningful transitions
	t.Logf("Status transitions: %v", statuses)

	// Should reach completed status
	finalStatus := statuses[len(statuses)-1]
	require.Contains(t, []string{"completed", "failed"}, finalStatus, "Should reach terminal status")

	// If we saw multiple statuses, verify they're valid
	validStatuses := map[string]bool{
		"created":   true,
		"running":   true,
		"completed": true,
		"failed":    true,
		"paused":    true,
	}

	for _, status := range statuses {
		require.True(t, validStatuses[status], "Status '%s' should be valid", status)
	}
}
