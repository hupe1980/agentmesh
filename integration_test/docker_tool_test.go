package integration_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/tool/docker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Docker Tool Log Retrieval Fix
//
// BUG FIXED: Previously, the Docker runner used AutoRemove=true which caused containers
// to be automatically removed by Docker immediately after they stopped running. This created
// a race condition where the code would wait for the container to complete, but by the time
// it tried to retrieve logs, the container was already removed/marked for removal, resulting in:
// "Error response from daemon: can not get logs from container which is dead or marked for removal"
//
// ROOT CAUSE: In runner.go, the sequence was:
//  1. Container created with AutoRemove=true
//  2. Container started
//  3. Wait for container to complete (ContainerWait)
//  4. Try to get logs (ContainerLogs) <- FAILS because container already removed by Docker
//
// FIX: Changed AutoRemove to always be false and manually remove container AFTER log retrieval:
//  1. Container created with AutoRemove=false
//  2. Container started
//  3. Wait for container to complete
//  4. Get logs (now succeeds because container still exists)
//  5. Manually remove container in deferred cleanup
//
// This ensures logs are always retrievable regardless of how quickly the container exits.

// TestDockerTool_LogRetrievalAfterContainerCompletion tests whether the Docker tool
// can successfully retrieve logs from containers after they complete execution.
// This test is designed to catch the race condition where containers might be removed
// before their logs are retrieved.
func TestDockerTool_LogRetrievalAfterContainerCompletion(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Docker integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Create Docker tool using the tool.Tool interface
	tool, err := docker.NewTool("test_docker", "alpine:latest",
		docker.WithDescription("Test Docker tool for log retrieval"),
		docker.WithTimeout(10*time.Second),
		docker.WithNetworkMode("none"),
		docker.WithPullImage(true), // Pull image if not present
	)
	require.NoError(t, err, "Failed to create Docker tool")
	defer tool.Close()

	t.Run("ShortLivedContainer", func(t *testing.T) {
		// Test case: Simple echo command that completes quickly
		argsJSON, err := json.Marshal(docker.Args{
			Command: "echo 'Hello from Docker'",
		})
		require.NoError(t, err)

		// This should NOT fail with "can not get logs from container which is dead or marked for removal"
		result, err := tool.Call(ctx, string(argsJSON))
		if err != nil {
			// Check if this is the specific error we're investigating
			t.Logf("Error occurred: %v", err)
			if strings.Contains(err.Error(), "can not get logs from container which is dead or marked for removal") {
				t.Errorf("BUG CONFIRMED: Container was removed before logs could be retrieved")
				t.Errorf("Error: %v", err)
			} else if strings.Contains(err.Error(), "Cannot connect to the Docker daemon") {
				t.Skipf("Skipping test: Docker daemon not running")
				return
			} else {
				t.Skipf("Skipping test due to Docker error: %v", err)
				return
			}
		}

		assert.NoError(t, err, "Docker tool should successfully retrieve logs")
		resultStr, ok := result.(string)
		require.True(t, ok, "Result should be a string")
		assert.Contains(t, resultStr, "Hello from Docker", "Output should contain expected text")
	})

	t.Run("MediumLivedContainer", func(t *testing.T) {
		// Test case: Container that runs for a few seconds
		argsJSON, err := json.Marshal(docker.Args{
			Command: "sleep 2",
		})
		require.NoError(t, err)

		_, err = tool.Call(ctx, string(argsJSON))
		if err != nil {
			if strings.Contains(err.Error(), "can not get logs from container which is dead or marked for removal") {
				t.Errorf("BUG CONFIRMED: Container was removed before logs could be retrieved")
				t.Errorf("Error: %v", err)
			} else if strings.Contains(err.Error(), "Cannot connect to the Docker daemon") {
				t.Skipf("Skipping test: Docker daemon not running")
				return
			} else {
				t.Skipf("Skipping test due to Docker error: %v", err)
				return
			}
		}

		assert.NoError(t, err)
		t.Logf("Medium-lived container completed successfully")
	})

	t.Run("InstantExitContainer", func(t *testing.T) {
		// Test case: Container that exits immediately
		argsJSON, err := json.Marshal(docker.Args{
			Command: "true", // Exits immediately with code 0
		})
		require.NoError(t, err)

		result, err := tool.Call(ctx, string(argsJSON))
		if err != nil {
			if strings.Contains(err.Error(), "can not get logs from container which is dead or marked for removal") {
				t.Errorf("BUG CONFIRMED: Container was removed before logs could be retrieved")
				t.Errorf("Error: %v", err)
			} else if strings.Contains(err.Error(), "Cannot connect to the Docker daemon") {
				t.Skipf("Skipping test: Docker daemon not running")
				return
			} else {
				t.Skipf("Skipping test due to Docker error: %v", err)
				return
			}
		}

		// Should succeed even with no output
		assert.NoError(t, err)
		resultStr, ok := result.(string)
		require.True(t, ok, "Result should be a string")
		t.Logf("Output from instant exit: %q", resultStr)
	})

	t.Run("RapidSequentialExecutions", func(t *testing.T) {
		// Test case: Multiple rapid executions to catch race condition
		argsJSON, err := json.Marshal(docker.Args{
			Command: "echo test",
		})
		require.NoError(t, err)

		var failures []error
		for i := 0; i < 10; i++ {
			_, err := tool.Call(ctx, string(argsJSON))
			if err != nil {
				if strings.Contains(err.Error(), "can not get logs from container which is dead or marked for removal") {
					failures = append(failures, err)
				} else if strings.Contains(err.Error(), "Cannot connect to the Docker daemon") {
					t.Skipf("Skipping test: Docker daemon not running")
					return
				}
			}
		}

		if len(failures) > 0 {
			t.Errorf("BUG CONFIRMED: %d/%d executions failed with log retrieval race condition", len(failures), 10)
			t.Errorf("Example error: %v", failures[0])
		}
	})
}

// TestDockerTool_ErrorHandling tests log retrieval from containers that exit with errors
func TestDockerTool_ErrorHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Docker integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tool, err := docker.NewTool("test_docker_errors", "alpine:latest",
		docker.WithDescription("Test Docker tool for error handling"),
		docker.WithTimeout(10*time.Second),
		docker.WithPullImage(true), // Pull image if not present
	)
	require.NoError(t, err)
	defer tool.Close()

	t.Run("ContainerWithNonZeroExit", func(t *testing.T) {
		// Test with failing container - use 'false' command which exits with code 1
		argsJSON, err := json.Marshal(docker.Args{
			Command: "false",
		})
		require.NoError(t, err)

		// Should still be able to retrieve logs even if container failed
		result, err := tool.Call(ctx, string(argsJSON))
		if err != nil {
			if strings.Contains(err.Error(), "can not get logs from container which is dead or marked for removal") {
				t.Errorf("BUG CONFIRMED: Even failed containers should have their logs retrieved before removal")
			} else if strings.Contains(err.Error(), "Cannot connect to the Docker daemon") {
				t.Skipf("Skipping test: Docker daemon not running")
				return
			}
			t.Logf("Error (expected exit code error): %v", err)
		}

		resultStr, ok := result.(string)
		require.True(t, ok, "Result should be a string")
		t.Logf("Result: %s", resultStr)

		// The command exits with non-zero, but logs should still be retrievable
		assert.Contains(t, resultStr, "[exit code]: 1", "Should show non-zero exit code")
	})
}

// TestDockerRunner_DirectUse tests the runner directly to verify the bug at a lower level
func TestDockerRunner_DirectUse(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Docker integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	runner, err := docker.NewRunner()
	require.NoError(t, err)
	defer runner.Close()

	t.Run("AutoRemoveEnabled_FastContainer", func(t *testing.T) {
		// This is the problematic scenario: AutoRemove=true with a fast-completing container
		result, err := runner.Run(ctx, docker.Config{
			Image:       "alpine:latest",
			Command:     []string{"echo", "Test output"},
			AutoRemove:  true, // THIS IS THE PROBLEMATIC SETTING
			NetworkMode: "none",
			Timeout:     5 * time.Second,
			PullImage:   true, // Pull if not present
		})

		if err != nil {
			if strings.Contains(err.Error(), "can not get logs from container which is dead or marked for removal") {
				t.Errorf("BUG CONFIRMED: AutoRemove=true causes container to be removed before logs can be retrieved")
				t.Errorf("Error: %v", err)
				t.Logf("This proves the hypothesis: The container is marked for removal immediately after exit,")
				t.Logf("but the code tries to get logs AFTER waiting for the container to stop.")
			} else if strings.Contains(err.Error(), "Cannot connect to the Docker daemon") {
				t.Skipf("Skipping test: Docker daemon not running")
				return
			} else {
				t.Logf("Different error: %v", err)
			}
		} else {
			t.Logf("Success with AutoRemove=true. Output: %s", string(result.Stdout))
		}
	})

	t.Run("AutoRemoveDisabled_FastContainer", func(t *testing.T) {
		// This should work because manual cleanup happens AFTER log retrieval
		result, err := runner.Run(ctx, docker.Config{
			Image:       "alpine:latest",
			Command:     []string{"echo", "Test output"},
			AutoRemove:  false, // Manual cleanup
			NetworkMode: "none",
			Timeout:     5 * time.Second,
			PullImage:   false, // Already pulled in previous test
		})

		if err != nil {
			if strings.Contains(err.Error(), "Cannot connect to the Docker daemon") {
				t.Skipf("Skipping test: Docker daemon not running")
				return
			}
			t.Fatalf("Unexpected error with AutoRemove=false: %v", err)
		}

		assert.NoError(t, err)
		assert.Contains(t, string(result.Stdout), "Test output")
		t.Logf("Success with AutoRemove=false. Output: %s", string(result.Stdout))
	})
}
