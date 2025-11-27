package viz

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	// stabilityStatusStable represents a stable test result
	stabilityStatusStable = "stable"
)

// TestRunner executes test suites and tracks results.
type TestRunner struct {
	server        *Server
	mu            sync.RWMutex
	history       map[string]*TestHistory // suiteID:testName -> history
	goldenManager *GoldenFileManager
}

// NewTestRunner creates a new test runner.
func NewTestRunner(server *Server) *TestRunner {
	// Default golden files directory
	goldenDir := "./testdata/golden"

	return &TestRunner{
		server:        server,
		history:       make(map[string]*TestHistory),
		goldenManager: NewGoldenFileManager(goldenDir),
	}
}

// SetGoldenDir sets the base directory for golden files.
func (tr *TestRunner) SetGoldenDir(dir string) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.goldenManager = NewGoldenFileManager(dir)
}

// RunSuite executes a test suite and returns results.
func (tr *TestRunner) RunSuite(ctx context.Context, suite *TestSuite, req TestRunRequest) (*TestRunResponse, error) {
	batchID := uuid.New().String()
	startTime := time.Now()

	// Filter tests
	tests := tr.filterTests(suite, req)

	response := &TestRunResponse{
		BatchID:    batchID,
		SuiteID:    suite.ID,
		TotalTests: len(tests),
		Results:    make([]TestRun, 0, len(tests)),
	}

	// Execute tests
	if req.Parallel > 1 {
		response.Results = tr.runParallel(ctx, suite, tests, req)
	} else {
		response.Results = tr.runSequential(ctx, suite, tests, req)
	}

	// Aggregate results
	for i := range response.Results {
		switch response.Results[i].Status {
		case TestStatusPassed:
			response.Passed++
		case TestStatusFailed:
			response.Failed++
		case TestStatusSkipped:
			response.Skipped++
		case TestStatusError:
			response.Errors++
		}
	}

	response.Duration = time.Since(startTime)

	return response, nil
}

// filterTests filters tests based on request criteria.
func (tr *TestRunner) filterTests(suite *TestSuite, req TestRunRequest) []TestCase {
	filtered := make([]TestCase, 0)

	for _, test := range suite.Tests {
		// Filter by specific test names
		if len(req.Tests) > 0 {
			found := false
			for _, name := range req.Tests {
				if test.Name == name {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		// Filter by tags
		if len(req.Tags) > 0 {
			hasTag := false
			for _, reqTag := range req.Tags {
				for _, testTag := range test.Tags {
					if reqTag == testTag {
						hasTag = true
						break
					}
				}
				if hasTag {
					break
				}
			}
			if !hasTag {
				continue
			}
		}

		filtered = append(filtered, test)
	}

	return filtered
}

// runSequential executes tests one at a time.
func (tr *TestRunner) runSequential(ctx context.Context, suite *TestSuite, tests []TestCase, req TestRunRequest) []TestRun {
	results := make([]TestRun, 0, len(tests))

	for _, test := range tests {
		result := tr.runTest(ctx, suite, test, req.UpdateGolden)
		results = append(results, result)

		// Update history
		tr.updateHistory(suite.ID, test.Name, result)
	}

	return results
}

// runParallel executes tests in parallel.
func (tr *TestRunner) runParallel(ctx context.Context, suite *TestSuite, tests []TestCase, req TestRunRequest) []TestRun {
	results := make([]TestRun, len(tests))
	var wg sync.WaitGroup

	sem := make(chan struct{}, req.Parallel)

	for i, test := range tests {
		wg.Add(1)
		go func(idx int, tc TestCase) {
			defer wg.Done()

			sem <- struct{}{}        // Acquire
			defer func() { <-sem }() // Release

			result := tr.runTest(ctx, suite, tc, req.UpdateGolden)
			results[idx] = result

			// Update history
			tr.updateHistory(suite.ID, tc.Name, result)
		}(i, test)
	}

	wg.Wait()

	return results
}

// runTest executes a single test case.
func (tr *TestRunner) runTest(ctx context.Context, suite *TestSuite, test TestCase, updateGolden bool) TestRun {
	runID := uuid.New().String()
	startTime := time.Now()

	result := TestRun{
		ID:        runID,
		SuiteID:   suite.ID,
		TestName:  test.Name,
		GraphID:   suite.GraphID,
		Input:     test.Input,
		Expected:  test.Expected,
		StartTime: startTime,
		Tags:      test.Tags,
	}

	// Apply timeout if specified
	testCtx := ctx
	if test.Timeout > 0 {
		var cancel context.CancelFunc
		testCtx, cancel = context.WithTimeout(ctx, test.Timeout)
		defer cancel()
	}

	// Execute graph
	graphRunID, err := tr.server.ExecuteGraph(testCtx, suite.GraphID, test.Input)
	if err != nil {
		result.Status = TestStatusError
		result.ErrorMsg = fmt.Sprintf("Failed to execute graph: %v", err)
		result.EndTime = time.Now()
		result.Duration = time.Since(startTime)
		return result
	}

	result.RunID = graphRunID

	// Wait for execution to complete
	actualOutput, execErr := tr.waitForCompletion(testCtx, graphRunID)

	result.EndTime = time.Now()
	result.Duration = time.Since(startTime)
	result.Actual = actualOutput

	if execErr != nil {
		result.Status = TestStatusError
		result.ErrorMsg = fmt.Sprintf("Graph execution error: %v", execErr)
		return result
	}

	// Get execution metrics
	result.TokensUsed, result.CostUSD = tr.getExecutionMetrics(graphRunID)

	// Determine expected output source
	var diffs []Diff

	switch {
	case test.Expected != nil:
		// Use inline expected output
		diffs = tr.compareOutputs(test.Expected, actualOutput)
	case test.GoldenFile != "" || tr.goldenManager.Exists(suite.ID, test.Name):
		// Use golden file comparison
		if updateGolden {
			// Update golden file mode
			if err := tr.goldenManager.Save(suite.ID, test.Name, actualOutput); err != nil {
				result.Status = TestStatusError
				result.ErrorMsg = fmt.Sprintf("Failed to update golden file: %v", err)
				return result
			}
			result.Status = TestStatusPassed
			result.Expected = actualOutput
			return result
		}

		// Load and compare with golden file
		golden, err := tr.goldenManager.Load(suite.ID, test.Name)
		if err != nil {
			result.Status = TestStatusError
			result.ErrorMsg = fmt.Sprintf("Failed to load golden file: %v", err)
			return result
		}

		result.Expected = golden
		diffs = tr.compareOutputs(golden, actualOutput)
	default:
		// No expected output, just check it ran successfully
		result.Status = TestStatusPassed
		return result
	}

	// Set result based on diffs
	result.Diffs = diffs
	if len(diffs) == 0 {
		result.Status = TestStatusPassed
	} else {
		result.Status = TestStatusFailed
	}

	return result
}

// waitForCompletion waits for graph execution to complete and returns output.
func (tr *TestRunner) waitForCompletion(ctx context.Context, runID string) (map[string]any, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	timeout := time.After(30 * time.Second) // Default timeout

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeout:
			return nil, fmt.Errorf("execution timeout")
		case <-ticker.C:
			// Check if run is complete
			tr.server.mu.RLock()
			_, active := tr.server.activeRuns[runID]
			tr.server.mu.RUnlock()

			if !active {
				// Run completed, get final output
				output := tr.getFinalOutput(runID)
				return output, nil
			}
		}
	}
}

// getFinalOutput retrieves the final output from a completed run.
func (tr *TestRunner) getFinalOutput(runID string) map[string]any {
	// Get events and extract final state
	events, _ := tr.server.eventStore.GetEvents(runID, 0)

	// Find the last state update or graph complete event
	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]
		if event.Type == EventGraphComplete || event.Type == EventStateUpdate {
			if event.Payload.StateAfter != nil {
				return event.Payload.StateAfter
			}
		}
	}

	return make(map[string]any)
}

// getExecutionMetrics retrieves token usage and cost from execution.
func (tr *TestRunner) getExecutionMetrics(runID string) (int, float64) {
	events, _ := tr.server.eventStore.GetEvents(runID, 0)

	totalTokens := 0
	totalCost := 0.0

	for i := range events {
		totalTokens += events[i].Payload.TotalTokens
		totalCost += events[i].Payload.EstCostUSD
	}

	return totalTokens, totalCost
}

// compareOutputs compares expected and actual outputs and returns differences.
func (tr *TestRunner) compareOutputs(expected, actual map[string]any) []Diff {
	diffs := make([]Diff, 0)

	// Check for missing keys
	for key, expectedVal := range expected {
		actualVal, exists := actual[key]

		if !exists {
			diffs = append(diffs, Diff{
				Path:     key,
				Expected: expectedVal,
				Actual:   nil,
				Type:     "removed",
			})
			continue
		}

		// Compare values
		if !tr.valuesEqual(expectedVal, actualVal) {
			diffs = append(diffs, Diff{
				Path:     key,
				Expected: expectedVal,
				Actual:   actualVal,
				Type:     "modified",
			})
		}
	}

	// Check for extra keys
	for key, actualVal := range actual {
		if _, exists := expected[key]; !exists {
			diffs = append(diffs, Diff{
				Path:     key,
				Expected: nil,
				Actual:   actualVal,
				Type:     "added",
			})
		}
	}

	return diffs
}

// valuesEqual checks if two values are equal.
func (tr *TestRunner) valuesEqual(a, b any) bool {
	// Deep equality check
	return reflect.DeepEqual(a, b)
}

// updateHistory updates test history with the new result.
func (tr *TestRunner) updateHistory(suiteID, testName string, result TestRun) {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	key := fmt.Sprintf("%s:%s", suiteID, testName)
	history, exists := tr.history[key]

	if !exists {
		history = &TestHistory{
			SuiteID:  suiteID,
			TestName: testName,
			Runs:     make([]TestRunSummary, 0),
		}
		tr.history[key] = history
	}

	// Add summary
	summary := TestRunSummary{
		ID:         result.ID,
		Status:     result.Status,
		Duration:   result.Duration,
		TokensUsed: result.TokensUsed,
		CostUSD:    result.CostUSD,
		Timestamp:  result.StartTime,
		DiffCount:  len(result.Diffs),
	}

	history.Runs = append(history.Runs, summary)
	history.TotalRuns++

	// Keep only last 50 runs
	if len(history.Runs) > 50 {
		history.Runs = history.Runs[len(history.Runs)-50:]
	}

	// Update statistics
	tr.calculateStatistics(history)

	// Update last passed/failed
	now := result.StartTime
	switch result.Status {
	case TestStatusPassed:
		history.LastPassed = &now
	case TestStatusFailed:
		history.LastFailed = &now
	}
}

// calculateStatistics computes statistics for test history.
func (tr *TestRunner) calculateStatistics(history *TestHistory) {
	if len(history.Runs) == 0 {
		return
	}

	passed := 0
	totalDuration := time.Duration(0)
	totalCost := 0.0

	for _, run := range history.Runs {
		if run.Status == TestStatusPassed {
			passed++
		}
		totalDuration += run.Duration
		totalCost += run.CostUSD
	}

	history.PassRate = float64(passed) / float64(len(history.Runs))
	history.AvgDuration = totalDuration / time.Duration(len(history.Runs))
	history.AvgCost = totalCost / float64(len(history.Runs))

	// Detect flaky tests (alternating pass/fail)
	if len(history.Runs) >= 3 {
		changes := 0
		for i := 1; i < len(history.Runs); i++ {
			if history.Runs[i].Status != history.Runs[i-1].Status {
				changes++
			}
		}
		history.Flaky = changes >= 2
	}

	// Determine trend
	history.Trend = tr.calculateTrend(history.Runs)
}

// calculateTrend determines if test is improving, stable, or degrading.
func (tr *TestRunner) calculateTrend(runs []TestRunSummary) string {
	if len(runs) < 5 {
		return stabilityStatusStable
	}

	// Look at last 5 runs
	recent := runs[len(runs)-5:]

	passed := 0
	for _, run := range recent {
		if run.Status == TestStatusPassed {
			passed++
		}
	}

	switch {
	case passed == 5:
		return stabilityStatusStable
	case passed >= 4:
		return "improving"
	case passed <= 1:
		return "degrading"
	}

	return stabilityStatusStable
}

// GetHistory returns test history for a specific test.
func (tr *TestRunner) GetHistory(suiteID, testName string) *TestHistory {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", suiteID, testName)
	history, exists := tr.history[key]

	if !exists {
		return nil
	}

	// Return a copy
	historyCopy := *history
	historyCopy.Runs = make([]TestRunSummary, len(history.Runs))
	copy(historyCopy.Runs, history.Runs)

	return &historyCopy
}

// CompareRuns compares two test runs.
func (tr *TestRunner) CompareRuns(baseRun, compareRun *TestRun) *TestComparisonResult {
	result := &TestComparisonResult{
		BaseRun: TestRunSummary{
			ID:         baseRun.ID,
			Status:     baseRun.Status,
			Duration:   baseRun.Duration,
			TokensUsed: baseRun.TokensUsed,
			CostUSD:    baseRun.CostUSD,
			Timestamp:  baseRun.StartTime,
			DiffCount:  len(baseRun.Diffs),
		},
		CompareRun: TestRunSummary{
			ID:         compareRun.ID,
			Status:     compareRun.Status,
			Duration:   compareRun.Duration,
			TokensUsed: compareRun.TokensUsed,
			CostUSD:    compareRun.CostUSD,
			Timestamp:  compareRun.StartTime,
			DiffCount:  len(compareRun.Diffs),
		},
	}

	// Compare outputs
	result.OutputDiffs = tr.compareOutputs(baseRun.Actual, compareRun.Actual)

	// Performance comparison
	result.DurationDelta = compareRun.Duration - baseRun.Duration
	if baseRun.Duration > 0 {
		result.DurationPercent = float64(result.DurationDelta) / float64(baseRun.Duration) * 100
	}

	result.TokensDelta = compareRun.TokensUsed - baseRun.TokensUsed
	if baseRun.TokensUsed > 0 {
		result.TokensPercent = float64(result.TokensDelta) / float64(baseRun.TokensUsed) * 100
	}

	result.CostDelta = compareRun.CostUSD - baseRun.CostUSD
	if baseRun.CostUSD > 0 {
		result.CostPercent = result.CostDelta / baseRun.CostUSD * 100
	}

	// Result comparison
	result.StatusChanged = baseRun.Status != compareRun.Status
	result.ImprovedQuality = len(compareRun.Diffs) < len(baseRun.Diffs)
	result.Regression = (baseRun.Status == TestStatusPassed && compareRun.Status == TestStatusFailed) ||
		(len(compareRun.Diffs) > len(baseRun.Diffs))

	return result
}

// SerializeTestRun converts a test run to JSON.
func (tr *TestRunner) SerializeTestRun(run *TestRun) ([]byte, error) {
	return json.MarshalIndent(run, "", "  ")
}

// DeserializeTestRun converts JSON to a test run.
func (tr *TestRunner) DeserializeTestRun(data []byte) (*TestRun, error) {
	var run TestRun
	if err := json.Unmarshal(data, &run); err != nil {
		return nil, err
	}
	return &run, nil
}
