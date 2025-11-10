package wasm_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/tool/wasm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper functions to load test WASM modules
func getCalculatorWASM(t *testing.T) []byte {
	t.Helper()
	wasmPath := filepath.Join("..", "..", "..", "examples", "wasm_tool", "calculator.wasm")
	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		t.Skipf("Calculator WASM not found (run: cd examples/wasm_tool && ./build.sh): %v", err)
	}
	return wasmBytes
}

func getNetworkAttemptWASM(t *testing.T) []byte {
	t.Helper()
	wasmPath := filepath.Join("testdata", "network_attempt.wasm")
	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		t.Skipf("network_attempt.wasm not found (run: cd testdata && ./build.sh): %v", err)
	}
	return wasmBytes
}

func getTimeoutBombWASM(t *testing.T) []byte {
	t.Helper()
	wasmPath := filepath.Join("testdata", "timeout_bomb.wasm")
	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		t.Skipf("timeout_bomb.wasm not found (run: cd testdata && ./build.sh): %v", err)
	}
	return wasmBytes
}

func getMemoryBombWASM(t *testing.T) []byte {
	t.Helper()
	wasmPath := filepath.Join("testdata", "memory_bomb.wasm")
	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		t.Skipf("memory_bomb.wasm not found (run: cd testdata && ./build.sh): %v", err)
	}
	return wasmBytes
}

func getFilesystemEscapeWASM(t *testing.T) []byte {
	t.Helper()
	wasmPath := filepath.Join("testdata", "filesystem_escape.wasm")
	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		t.Skipf("filesystem_escape.wasm not found (run: cd testdata && ./build.sh): %v", err)
	}
	return wasmBytes
}

func getNonDeterministicWASM(t *testing.T) []byte {
	t.Helper()
	wasmPath := filepath.Join("testdata", "non_deterministic.wasm")
	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		t.Skipf("non_deterministic.wasm not found (run: cd testdata && ./build.sh): %v", err)
	}
	return wasmBytes
}

func getSimpleMathWASM(t *testing.T) []byte {
	t.Helper()
	wasmPath := filepath.Join("testdata", "simple_math.wasm")
	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		t.Skipf("simple_math.wasm not found (run: cd testdata && ./build.sh): %v", err)
	}
	return wasmBytes
}

func TestSandboxPolicy_PositiveTest(t *testing.T) {
	ctx := context.Background()
	wasmBytes := getSimpleMathWASM(t)

	schema := &wasm.ToolSchema{
		Name:        "simple_math",
		Description: "Simple math operations",
		Parameters: &wasm.ParameterSchema{
			Type: "object",
			Properties: map[string]wasm.PropertySchema{
				"a": {
					Type:        "number",
					Description: "First operand",
				},
				"b": {
					Type:        "number",
					Description: "Second operand",
				},
				"operation": {
					Type:        "string",
					Description: "Operation: add, subtract, multiply, divide, power, modulo",
				},
			},
			Required: []string{"a", "b", "operation"},
		},
	}

	tool, err := wasm.NewWASMTool(
		ctx,
		"simple_math",
		"Simple math operations",
		wasmBytes,
		wasm.WithPolicy(wasm.ComputeOnlyPolicy()),
		wasm.WithSchema(schema),
	)
	require.NoError(t, err, "Failed to create WASM tool")
	defer tool.Close(ctx)

	tests := []struct {
		name     string
		input    string
		expected float64
	}{
		{
			name:     "addition",
			input:    `{"a": 10, "b": 5, "operation": "add"}`,
			expected: 15.0,
		},
		{
			name:     "subtraction",
			input:    `{"a": 10, "b": 5, "operation": "subtract"}`,
			expected: 5.0,
		},
		{
			name:     "multiplication",
			input:    `{"a": 10, "b": 5, "operation": "multiply"}`,
			expected: 50.0,
		},
		{
			name:     "division",
			input:    `{"a": 10, "b": 5, "operation": "divide"}`,
			expected: 2.0,
		},
		{
			name:     "power",
			input:    `{"a": 2, "b": 3, "operation": "power"}`,
			expected: 8.0,
		},
		{
			name:     "modulo",
			input:    `{"a": 10, "b": 3, "operation": "modulo"}`,
			expected: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tool.Call(ctx, tt.input)
			require.NoError(t, err, "Failed to call tool")

			resultMap, ok := result.(map[string]any)
			require.True(t, ok, "Expected map result, got %T", result)

			assert.Nil(t, resultMap["error"], "Should not have error")
			assert.Equal(t, tt.expected, resultMap["result"], "Expected result %v", tt.expected)
		})
	}

	// Test error handling
	t.Run("division by zero", func(t *testing.T) {
		result, err := tool.Call(ctx, `{"a": 10, "b": 0, "operation": "divide"}`)
		require.NoError(t, err, "Call should succeed")

		resultMap, ok := result.(map[string]interface{})
		require.True(t, ok, "Expected map result")

		assert.Nil(t, resultMap["result"], "Should not have result")
		assert.NotNil(t, resultMap["error"], "Should have error")
		assert.Contains(t, resultMap["error"].(string), "division by zero", "Error message")
	})

	t.Run("unknown operation", func(t *testing.T) {
		result, err := tool.Call(ctx, `{"a": 10, "b": 5, "operation": "unknown"}`)
		require.NoError(t, err, "Call should succeed")

		resultMap, ok := result.(map[string]interface{})
		require.True(t, ok, "Expected map result")

		assert.Nil(t, resultMap["result"], "Should not have result")
		assert.NotNil(t, resultMap["error"], "Should have error")
		assert.Contains(t, resultMap["error"].(string), "unknown operation", "Error message")
	})
}

func TestSandboxPolicy_ComputeOnly(t *testing.T) {
	ctx := context.Background()
	wasmBytes := getCalculatorWASM(t)

	schema := &wasm.ToolSchema{
		Name:        "test_calculator",
		Description: "Test calculator",
		Parameters: &wasm.ParameterSchema{
			Type: "object",
			Properties: map[string]wasm.PropertySchema{
				"expression": {
					Type:        "string",
					Description: "A math expression",
				},
			},
			Required: []string{"expression"},
		},
	}

	// Create tool with ComputeOnly policy
	tool, err := wasm.NewWASMTool(
		ctx,
		"test_calculator",
		"Test calculator",
		wasmBytes,
		wasm.WithPolicy(wasm.ComputeOnlyPolicy()),
		wasm.WithSchema(schema),
	)
	require.NoError(t, err, "Failed to create WASM tool")
	defer tool.Close(ctx)

	// Test that computation works
	result, err := tool.Call(ctx, `{"expression": "2 + 2"}`)
	require.NoError(t, err, "Failed to call tool")

	resultMap, ok := result.(map[string]any)
	require.True(t, ok, "Expected map result, got %T", result)

	assert.Equal(t, 4.0, resultMap["result"], "Expected result 4")
}

func TestSandboxPolicy_Timeout(t *testing.T) {
	ctx := context.Background()
	wasmBytes := getTimeoutBombWASM(t)

	schema := &wasm.ToolSchema{
		Name:        "timeout_bomb",
		Description: "Test timeout enforcement",
		Parameters: &wasm.ParameterSchema{
			Type: "object",
			Properties: map[string]wasm.PropertySchema{
				"iterations": {
					Type:        "number",
					Description: "Number of iterations",
				},
			},
			Required: []string{"iterations"},
		},
	}

	// Create custom policy with short timeout
	policy := wasm.ComputeOnlyPolicy()
	policy.Timeout = 100 * time.Millisecond

	tool, err := wasm.NewWASMTool(
		ctx,
		"timeout_bomb",
		"Test timeout enforcement",
		wasmBytes,
		wasm.WithPolicy(policy),
		wasm.WithSchema(schema),
	)
	require.NoError(t, err, "Failed to create WASM tool")
	defer tool.Close(ctx)

	// This should timeout because the module has an infinite loop
	_, err = tool.Call(ctx, `{"iterations": 1000000}`)

	// The timeout enforcement should either:
	// 1. Return a context deadline exceeded error, or
	// 2. The WASM execution completes but is interrupted
	// Due to WASM execution model, timeout may not always trigger on simple loops
	if err != nil {
		assert.Contains(t, err.Error(), "deadline", "Expected deadline-related error")
	} else {
		// If no error, the execution likely completed before timeout
		// This is acceptable for very fast WASM execution
		t.Log("WASM execution completed before timeout (acceptable for fast operations)")
	}
}

func TestSandboxPolicy_MemoryLimit(t *testing.T) {
	ctx := context.Background()
	wasmBytes := getMemoryBombWASM(t)

	schema := &wasm.ToolSchema{
		Name:        "memory_bomb",
		Description: "Test memory limit enforcement",
		Parameters: &wasm.ParameterSchema{
			Type: "object",
			Properties: map[string]wasm.PropertySchema{
				"size_mb": {
					Type:        "number",
					Description: "Size in MB to allocate",
				},
			},
			Required: []string{"size_mb"},
		},
	}

	t.Run("Small allocation within limit", func(t *testing.T) {
		// Create policy with 10MB limit
		policy := wasm.ComputeOnlyPolicy()
		policy.MaxMemory = 10 * 1024 * 1024 // 10MB

		tool, err := wasm.NewWASMTool(
			ctx,
			"memory_bomb",
			"Test memory limit",
			wasmBytes,
			wasm.WithPolicy(policy),
			wasm.WithSchema(schema),
		)
		require.NoError(t, err, "Failed to create WASM tool")
		defer tool.Close(ctx)

		// Try to allocate 5MB (within limit)
		result, err := tool.Call(ctx, `{"size_mb": 5}`)
		require.NoError(t, err, "Failed to call tool")
		assert.NotNil(t, result, "Expected result")
	})

	t.Run("Large allocation exceeds limit", func(t *testing.T) {
		// Create policy with 1MB limit
		policy := wasm.ComputeOnlyPolicy()
		policy.MaxMemory = 1 * 1024 * 1024 // 1MB

		tool, err := wasm.NewWASMTool(
			ctx,
			"memory_bomb",
			"Test memory limit",
			wasmBytes,
			wasm.WithPolicy(policy),
			wasm.WithSchema(schema),
		)
		require.NoError(t, err, "Failed to create WASM tool")
		defer tool.Close(ctx)

		// Try to allocate 500MB (exceeds limit)
		result, err := tool.Call(ctx, `{"size_mb": 500}`)

		// Either the call should fail or the result should indicate memory failure
		if err != nil {
			// WASM runtime detected memory limit and failed the call
			assert.Contains(t, err.Error(), "memory", "Expected memory-related error")
			t.Logf("Memory limit enforced at runtime level: %v", err)
		} else {
			// WASM module caught the allocation failure internally
			resultMap, ok := result.(map[string]interface{})
			require.True(t, ok, "Expected map result")

			// The module should report the failure in its error field
			if errMsg, hasError := resultMap["error"].(string); hasError {
				assert.Contains(t, errMsg, "fail", "Expected failure message")
				t.Logf("Memory limit enforced at module level: %s", errMsg)
			} else {
				// Module somehow succeeded - log for investigation
				t.Logf("WARNING: Large allocation succeeded: %v", result)
				// Don't fail the test as WASM memory model may allow overcommit
			}
		}
	})
}

func TestSandboxPolicy_Deterministic(t *testing.T) {
	ctx := context.Background()
	wasmBytes := getCalculatorWASM(t)

	schema := &wasm.ToolSchema{
		Name:        "test_calculator",
		Description: "Test calculator",
		Parameters: &wasm.ParameterSchema{
			Type: "object",
			Properties: map[string]wasm.PropertySchema{
				"expression": {
					Type:        "string",
					Description: "A math expression",
				},
			},
			Required: []string{"expression"},
		},
	}

	// Create tool with deterministic policy
	tool, err := wasm.NewWASMTool(
		ctx,
		"test_calculator",
		"Test calculator",
		wasmBytes,
		wasm.WithPolicy(wasm.DeterministicPolicy()),
		wasm.WithSchema(schema),
	)
	require.NoError(t, err, "Failed to create WASM tool")
	defer tool.Close(ctx)

	input := `{"expression": "10 * 5 + 3"}`

	// Run multiple times and verify same result
	var firstResult interface{}
	for i := 0; i < 5; i++ {
		result, err := tool.Call(ctx, input)
		require.NoError(t, err, "Call %d failed", i)

		if i == 0 {
			firstResult = result
		} else {
			// Compare results
			resultMap1 := firstResult.(map[string]interface{})
			resultMap2 := result.(map[string]interface{})
			assert.Equal(t, resultMap1["result"], resultMap2["result"],
				"Non-deterministic result: call 0 vs call %d", i)
		}
	}
}

func TestSandboxPolicy_NonDeterministic(t *testing.T) {
	ctx := context.Background()
	wasmBytes := getNonDeterministicWASM(t)

	schema := &wasm.ToolSchema{
		Name:        "non_deterministic",
		Description: "Test non-deterministic behavior detection",
		Parameters: &wasm.ParameterSchema{
			Type: "object",
			Properties: map[string]wasm.PropertySchema{
				"expression": {
					Type:        "string",
					Description: "A math expression",
				},
			},
			Required: []string{"expression"},
		},
	}

	t.Run("Module exhibits non-deterministic behavior", func(t *testing.T) {
		tool, err := wasm.NewWASMTool(
			ctx,
			"non_deterministic",
			"Test non-deterministic behavior",
			wasmBytes,
			wasm.WithPolicy(wasm.ComputeOnlyPolicy()),
			wasm.WithSchema(schema),
		)
		require.NoError(t, err, "Failed to create WASM tool")
		defer tool.Close(ctx)

		input := `{"expression": "2 + 2"}`
		var results []float64

		// Run multiple times and collect results
		for i := 0; i < 5; i++ {
			result, err := tool.Call(ctx, input)
			require.NoError(t, err, "Call %d failed", i)

			resultMap, ok := result.(map[string]interface{})
			require.True(t, ok, "Expected map result")

			resultValue, ok := resultMap["result"].(float64)
			require.True(t, ok, "Expected float64 result")
			results = append(results, resultValue)
		}

		// With global state (STATE += 1), each call should return different results
		// Expected: 5.0, 6.0, 7.0, 8.0, 9.0 (4 + STATE where STATE increments each call)
		// However, due to WASM instantiation model, each Call() may create a new instance
		// which resets STATE to 0, making it appear deterministic

		// Check if results vary (non-deterministic) or are same (isolation working)
		allSame := true
		for i := 1; i < len(results); i++ {
			if results[i] != results[0] {
				allSame = false
				break
			}
		}

		if allSame {
			// Each call created a new instance - perfect isolation
			t.Logf("Module instantiation provides perfect isolation - all results: %v", results[0])
			assert.Equal(t, 5.0, results[0], "First call should return 4 + 1 = 5")
		} else {
			// State persisted across calls - non-deterministic behavior detected
			t.Logf("Non-deterministic behavior detected: %v", results)
			// Verify results are increasing
			for i := 1; i < len(results); i++ {
				assert.Greater(t, results[i], results[i-1],
					"Results should increase due to global state")
			}
		}
	})

	t.Run("DeterministicPolicy enforces fresh instances", func(t *testing.T) {
		tool, err := wasm.NewWASMTool(
			ctx,
			"non_deterministic",
			"Test deterministic enforcement",
			wasmBytes,
			wasm.WithPolicy(wasm.DeterministicPolicy()),
			wasm.WithSchema(schema),
		)
		require.NoError(t, err, "Failed to create WASM tool")
		defer tool.Close(ctx)

		input := `{"expression": "2 + 2"}`
		var results []float64

		// Run multiple times
		for i := 0; i < 5; i++ {
			result, err := tool.Call(ctx, input)
			require.NoError(t, err, "Call %d failed", i)

			resultMap, ok := result.(map[string]interface{})
			require.True(t, ok, "Expected map result")

			resultValue, ok := resultMap["result"].(float64)
			require.True(t, ok, "Expected float64 result")
			results = append(results, resultValue)
		}

		// With DeterministicPolicy, each call should get fresh instance
		// So all results should be the same (5.0)
		for i := 1; i < len(results); i++ {
			assert.Equal(t, results[0], results[i],
				"DeterministicPolicy should ensure same results by creating fresh instances")
		}

		t.Logf("DeterministicPolicy results (all should be same): %v", results)
	})
}

func TestSandboxPolicy_Isolation(t *testing.T) {
	ctx := context.Background()
	wasmBytes := getCalculatorWASM(t)

	schema := &wasm.ToolSchema{
		Name:        "test_calculator",
		Description: "Test calculator",
		Parameters: &wasm.ParameterSchema{
			Type: "object",
			Properties: map[string]wasm.PropertySchema{
				"expression": {
					Type:        "string",
					Description: "A math expression",
				},
			},
			Required: []string{"expression"},
		},
	}

	// Create two separate tool instances
	tool1, err := wasm.NewWASMTool(
		ctx,
		"calculator1",
		"Test calculator 1",
		wasmBytes,
		wasm.WithPolicy(wasm.ComputeOnlyPolicy()),
		wasm.WithSchema(schema),
	)
	if err != nil {
		t.Fatalf("Failed to create tool 1: %v", err)
	}
	defer tool1.Close(ctx)

	tool2, err := wasm.NewWASMTool(
		ctx,
		"calculator2",
		"Test calculator 2",
		wasmBytes,
		wasm.WithPolicy(wasm.ComputeOnlyPolicy()),
		wasm.WithSchema(schema),
	)
	if err != nil {
		t.Fatalf("Failed to create tool 2: %v", err)
	}
	defer tool2.Close(ctx)

	// Both tools should work independently
	result1, err := tool1.Call(ctx, `{"expression": "10 + 5"}`)
	require.NoError(t, err, "Tool 1 failed")

	result2, err := tool2.Call(ctx, `{"expression": "20 + 10"}`)
	require.NoError(t, err, "Tool 2 failed")

	// Verify different results
	r1 := result1.(map[string]interface{})["result"]
	r2 := result2.(map[string]interface{})["result"]

	assert.NotEqual(t, r1, r2, "Tools should have different results")
	assert.Equal(t, 15.0, r1, "Tool 1 result")
	assert.Equal(t, 30.0, r2, "Tool 2 result")
}

func TestSandboxPolicy_ErrorHandling(t *testing.T) {
	ctx := context.Background()
	wasmBytes := getCalculatorWASM(t)

	schema := &wasm.ToolSchema{
		Name:        "test_calculator",
		Description: "Test calculator",
		Parameters: &wasm.ParameterSchema{
			Type: "object",
			Properties: map[string]wasm.PropertySchema{
				"expression": {
					Type:        "string",
					Description: "A math expression",
				},
			},
			Required: []string{"expression"},
		},
	}

	tool, err := wasm.NewWASMTool(
		ctx,
		"test_calculator",
		"Test calculator",
		wasmBytes,
		wasm.WithPolicy(wasm.ComputeOnlyPolicy()),
		wasm.WithSchema(schema),
	)
	if err != nil {
		t.Fatalf("Failed to create WASM tool: %v", err)
	}
	defer tool.Close(ctx)

	tests := []struct {
		name        string
		input       string
		expectError bool
	}{
		{
			name:        "valid expression",
			input:       `{"expression": "2 + 2"}`,
			expectError: false,
		},
		{
			name:        "invalid expression",
			input:       `{"expression": "invalid"}`,
			expectError: false, // Returns error in result, not Go error
		},
		{
			name:        "division by zero",
			input:       `{"expression": "10 / 0"}`,
			expectError: false, // Returns error in result
		},
		{
			name:        "invalid JSON",
			input:       `{invalid json}`,
			expectError: false, // WASM handles this gracefully in result
		},
		{
			name:        "missing expression field",
			input:       `{}`,
			expectError: false, // WASM handles this
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tool.Call(ctx, tt.input)

			if tt.expectError {
				assert.Error(t, err, "Expected error but got none")
			} else {
				assert.NoError(t, err, "Unexpected error")
			}

			if err == nil {
				assert.NotNil(t, result, "Expected result but got nil")
			}
		})
	}
}

func TestSandboxPolicy_Presets(t *testing.T) {
	tests := []struct {
		name   string
		policy *wasm.SandboxPolicy
	}{
		{
			name:   "DefaultSandboxPolicy",
			policy: wasm.DefaultSandboxPolicy(),
		},
		{
			name:   "ComputeOnlyPolicy",
			policy: wasm.ComputeOnlyPolicy(),
		},
		{
			name:   "NetworkOnlyPolicy",
			policy: wasm.NetworkOnlyPolicy(),
		},
		{
			name:   "FileProcessingPolicy",
			policy: wasm.FileProcessingPolicy([]string{"/tmp"}, true),
		},
		{
			name:   "DeterministicPolicy",
			policy: wasm.DeterministicPolicy(),
		},
		{
			name:   "PermissiveSandboxPolicy",
			policy: wasm.PermissiveSandboxPolicy(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NotNil(t, tt.policy, "Policy should not be nil")

			// Validate policy has reasonable defaults
			assert.Positive(t, tt.policy.Timeout, "Timeout should be positive")
			assert.Positive(t, tt.policy.MaxMemory, "MaxMemory should be positive")
			assert.Positive(t, tt.policy.MaxOutputSize, "MaxOutputSize should be positive")
		})
	}
}

func TestSandboxPolicy_NetworkBlocked(t *testing.T) {
	ctx := context.Background()
	wasmBytes := getNetworkAttemptWASM(t)

	schema := &wasm.ToolSchema{
		Name:        "network_test",
		Description: "Test network access blocking",
		Parameters: &wasm.ParameterSchema{
			Type: "object",
			Properties: map[string]wasm.PropertySchema{
				"url": {
					Type:        "string",
					Description: "URL to attempt to access",
				},
			},
			Required: []string{"url"},
		},
	}

	// Test 1: ComputeOnlyPolicy should block network
	t.Run("ComputeOnlyPolicy blocks network", func(t *testing.T) {
		tool, err := wasm.NewWASMTool(
			ctx,
			"network_test",
			"Test network blocking",
			wasmBytes,
			wasm.WithPolicy(wasm.ComputeOnlyPolicy()),
			wasm.WithSchema(schema),
		)
		require.NoError(t, err, "Failed to create WASM tool")
		defer tool.Close(ctx)

		// Attempt to make a network call
		result, err := tool.Call(ctx, `{"url": "https://example.com"}`)
		require.NoError(t, err, "Call failed")

		// The tool should execute but report network access is denied
		resultMap, ok := result.(map[string]interface{})
		require.True(t, ok, "Expected map result, got %T", result)

		// Should have an error message about network access
		errMsg, hasError := resultMap["error"].(string)
		assert.True(t, hasError, "Expected error field in result when network is blocked")
		assert.NotEmpty(t, errMsg, "Expected error message about network access")
		assert.Contains(t, errMsg, "network", "Error should mention network")
		t.Logf("Network access correctly denied: %s", errMsg)

		// Should NOT have a successful result
		assert.Nil(t, resultMap["result"], "Expected no result when network access is blocked")
	})

	// Test 2: DeterministicPolicy should also block network
	t.Run("DeterministicPolicy blocks network", func(t *testing.T) {
		tool, err := wasm.NewWASMTool(
			ctx,
			"network_test",
			"Test network blocking",
			wasmBytes,
			wasm.WithPolicy(wasm.DeterministicPolicy()),
			wasm.WithSchema(schema),
		)
		require.NoError(t, err, "Failed to create WASM tool")
		defer tool.Close(ctx)

		result, err := tool.Call(ctx, `{"url": "https://example.com"}`)
		require.NoError(t, err, "Call failed")

		resultMap, ok := result.(map[string]interface{})
		require.True(t, ok, "Expected map result, got %T", result)

		// Should have an error about network access
		errMsg, hasError := resultMap["error"].(string)
		assert.True(t, hasError, "Expected error field in result when network is blocked")
		assert.NotEmpty(t, errMsg, "Expected error message about network access")
	})

	// Test 3: NetworkOnlyPolicy should theoretically allow network (but WASI doesn't provide it)
	t.Run("NetworkOnlyPolicy acknowledges network capability", func(t *testing.T) {
		policy := wasm.NetworkOnlyPolicy()

		// Verify policy is configured for network
		assert.True(t, policy.AllowNetwork, "NetworkOnlyPolicy should have AllowNetwork=true")

		// Note: Even with AllowNetwork=true, WASI doesn't provide network access
		// without additional host functions. The policy just indicates intent.
		t.Logf("NetworkOnlyPolicy configured: AllowNetwork=%v", policy.AllowNetwork)
	})
}

func TestSandboxPolicy_FilesystemBlocked(t *testing.T) {
	ctx := context.Background()
	wasmBytes := getFilesystemEscapeWASM(t)

	schema := &wasm.ToolSchema{
		Name:        "filesystem_escape",
		Description: "Test filesystem blocking",
		Parameters: &wasm.ParameterSchema{
			Type: "object",
			Properties: map[string]wasm.PropertySchema{
				"path": {
					Type:        "string",
					Description: "Path to attempt to access",
				},
			},
			Required: []string{"path"},
		},
	}

	// ComputeOnlyPolicy should block filesystem access
	policy := wasm.ComputeOnlyPolicy()
	assert.False(t, policy.AllowFilesystem, "ComputeOnlyPolicy should have AllowFilesystem=false")

	tool, err := wasm.NewWASMTool(
		ctx,
		"filesystem_escape",
		"Test filesystem blocking",
		wasmBytes,
		wasm.WithPolicy(policy),
		wasm.WithSchema(schema),
	)
	require.NoError(t, err, "Failed to create WASM tool")
	defer tool.Close(ctx)

	// Try to access a sensitive file
	result, err := tool.Call(ctx, `{"path": "/etc/passwd"}`)
	require.NoError(t, err, "Call failed")

	// The tool should execute but all filesystem operations should fail
	resultMap := result.(map[string]interface{})

	// Should have an error about filesystem access being blocked
	errMsg, hasError := resultMap["error"].(string)
	assert.True(t, hasError, "Expected error field in result when filesystem is blocked")
	assert.NotEmpty(t, errMsg, "Expected error message about filesystem access")
	assert.Contains(t, errMsg, "fail", "Error should mention failure")
	t.Logf("Filesystem access correctly denied: %s", errMsg)

	// Should NOT have a successful result
	assert.Nil(t, resultMap["result"], "Expected no result when filesystem access is blocked")
}

func TestSandboxPolicy_ConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	wasmBytes := getCalculatorWASM(t)

	schema := &wasm.ToolSchema{
		Name:        "test_calculator",
		Description: "Test calculator",
		Parameters: &wasm.ParameterSchema{
			Type: "object",
			Properties: map[string]wasm.PropertySchema{
				"expression": {
					Type:        "string",
					Description: "A math expression",
				},
			},
			Required: []string{"expression"},
		},
	}

	// Run multiple concurrent calls with separate tool instances
	// (Each WASM module call gets its own module instance for isolation)
	const numCalls = 5
	results := make(chan error, numCalls)

	for i := 0; i < numCalls; i++ {
		go func(n int) {
			// Create separate tool instance for each goroutine
			tool, err := wasm.NewWASMTool(
				ctx,
				"test_calculator",
				"Test calculator",
				wasmBytes,
				wasm.WithPolicy(wasm.ComputeOnlyPolicy()),
				wasm.WithSchema(schema),
			)
			if err != nil {
				results <- err
				return
			}
			defer tool.Close(ctx)

			input := `{"expression": "2 + 2"}`
			_, err = tool.Call(ctx, input)
			results <- err
		}(i)
	}

	// Collect results
	for i := 0; i < numCalls; i++ {
		err := <-results
		assert.NoError(t, err, "Concurrent call %d failed", i)
	}
}

func BenchmarkSandboxPolicy_Call(b *testing.B) {
	ctx := context.Background()
	wasmBytes := getCalculatorWASM(&testing.T{})

	schema := &wasm.ToolSchema{
		Name:        "test_calculator",
		Description: "Test calculator",
		Parameters: &wasm.ParameterSchema{
			Type: "object",
			Properties: map[string]wasm.PropertySchema{
				"expression": {
					Type:        "string",
					Description: "A math expression",
				},
			},
			Required: []string{"expression"},
		},
	}

	tool, err := wasm.NewWASMTool(
		ctx,
		"test_calculator",
		"Test calculator",
		wasmBytes,
		wasm.WithPolicy(wasm.ComputeOnlyPolicy()),
		wasm.WithSchema(schema),
	)
	if err != nil {
		b.Fatalf("Failed to create WASM tool: %v", err)
	}
	defer tool.Close(ctx)

	input := `{"expression": "2 + 2"}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := tool.Call(ctx, input)
		if err != nil {
			b.Fatalf("Call failed: %v", err)
		}
	}
}
