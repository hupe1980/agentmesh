package viz

import (
	"time"
)

// TestStatus represents the result of a test execution.
type TestStatus string

// Test status constants
const (
	TestStatusPassed  TestStatus = "passed"
	TestStatusFailed  TestStatus = "failed"
	TestStatusSkipped TestStatus = "skipped"
	TestStatusError   TestStatus = "error" // Test couldn't run
)

// TestRun represents a single test execution with its results.
type TestRun struct {
	ID       string `json:"id"`        // Unique test run ID
	SuiteID  string `json:"suite_id"`  // Test suite identifier
	TestName string `json:"test_name"` // Individual test name
	GraphID  string `json:"graph_id"`  // Graph being tested

	// Execution details
	Input    map[string]any `json:"input"`    // Test input
	Expected map[string]any `json:"expected"` // Expected output
	Actual   map[string]any `json:"actual"`   // Actual output
	Status   TestStatus     `json:"status"`   // Test result

	// Timing and resources
	StartTime  time.Time     `json:"start_time"`
	EndTime    time.Time     `json:"end_time"`
	Duration   time.Duration `json:"duration"`
	TokensUsed int           `json:"tokens_used"`
	CostUSD    float64       `json:"cost_usd"`

	// Comparison details
	Diffs    []Diff `json:"diffs,omitempty"` // Differences from expected
	ErrorMsg string `json:"error_msg,omitempty"`

	// Metadata
	RunID    string         `json:"run_id"` // Graph execution run ID
	Tags     []string       `json:"tags"`   // Test tags
	Metadata map[string]any `json:"metadata,omitempty"`
}

// TestSuite groups related tests together.
type TestSuite struct {
	ID          string `json:"id"`          // Suite identifier
	Name        string `json:"name"`        // Human-readable name
	Description string `json:"description"` // Suite description
	GraphID     string `json:"graph_id"`    // Graph to test

	// Test cases
	Tests []TestCase `json:"tests"` // Test cases in suite

	// Metadata
	Tags      []string  `json:"tags"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TestCase defines a single test case within a suite.
type TestCase struct {
	Name        string         `json:"name"`                  // Test name
	Description string         `json:"description"`           // What this tests
	Input       map[string]any `json:"input"`                 // Test input
	Expected    map[string]any `json:"expected"`              // Expected output
	GoldenFile  string         `json:"golden_file,omitempty"` // Path to golden file
	Timeout     time.Duration  `json:"timeout,omitempty"`     // Test timeout
	Tags        []string       `json:"tags"`
}

// Diff represents a difference between expected and actual values.
type Diff struct {
	Path     string `json:"path"`     // JSON path to the difference
	Expected any    `json:"expected"` // Expected value
	Actual   any    `json:"actual"`   // Actual value
	Type     string `json:"type"`     // Type of diff: added, removed, modified
}

// TestHistory tracks test runs over time for regression detection.
type TestHistory struct {
	SuiteID  string `json:"suite_id"`
	TestName string `json:"test_name"`

	// Historical data
	Runs      []TestRunSummary `json:"runs"`       // Recent runs
	TotalRuns int              `json:"total_runs"` // All-time run count

	// Statistics
	PassRate    float64       `json:"pass_rate"`    // Success rate (0-1)
	AvgDuration time.Duration `json:"avg_duration"` // Average execution time
	AvgCost     float64       `json:"avg_cost"`     // Average cost

	// Regression detection
	LastPassed *time.Time `json:"last_passed,omitempty"`
	LastFailed *time.Time `json:"last_failed,omitempty"`
	Flaky      bool       `json:"flaky"` // Inconsistent results
	Trend      string     `json:"trend"` // improving, stable, degrading
}

// TestRunSummary is a lightweight version of TestRun for history.
type TestRunSummary struct {
	ID         string        `json:"id"`
	Status     TestStatus    `json:"status"`
	Duration   time.Duration `json:"duration"`
	TokensUsed int           `json:"tokens_used"`
	CostUSD    float64       `json:"cost_usd"`
	Timestamp  time.Time     `json:"timestamp"`
	DiffCount  int           `json:"diff_count"` // Number of differences
}

// TestComparisonRequest requests comparison between test runs.
type TestComparisonRequest struct {
	BaseRunID    string `json:"base_run_id"`    // Base test run
	CompareRunID string `json:"compare_run_id"` // Run to compare
}

// TestComparisonResult shows differences between two test runs.
type TestComparisonResult struct {
	BaseRun    TestRunSummary `json:"base_run"`
	CompareRun TestRunSummary `json:"compare_run"`

	// Output comparison
	OutputDiffs []Diff `json:"output_diffs"`

	// Performance comparison
	DurationDelta   time.Duration `json:"duration_delta"`   // Time difference
	DurationPercent float64       `json:"duration_percent"` // % change
	TokensDelta     int           `json:"tokens_delta"`     // Token difference
	TokensPercent   float64       `json:"tokens_percent"`   // % change
	CostDelta       float64       `json:"cost_delta"`       // Cost difference
	CostPercent     float64       `json:"cost_percent"`     // % change

	// Result comparison
	StatusChanged   bool `json:"status_changed"`
	ImprovedQuality bool `json:"improved_quality"` // Fewer diffs
	Regression      bool `json:"regression"`       // New failure or more diffs
}

// TestRunRequest represents a request to run tests.
type TestRunRequest struct {
	SuiteID      string   `json:"suite_id"`           // Suite to run
	Tests        []string `json:"tests,omitempty"`    // Specific tests (empty = all)
	Tags         []string `json:"tags,omitempty"`     // Filter by tags
	UpdateGolden bool     `json:"update_golden"`      // Update golden files
	Parallel     int      `json:"parallel,omitempty"` // Parallel execution
}

// TestRunResponse returns test execution results.
type TestRunResponse struct {
	BatchID    string        `json:"batch_id"` // Batch run identifier
	SuiteID    string        `json:"suite_id"`
	TotalTests int           `json:"total_tests"`
	Passed     int           `json:"passed"`
	Failed     int           `json:"failed"`
	Skipped    int           `json:"skipped"`
	Errors     int           `json:"errors"`
	Duration   time.Duration `json:"duration"`
	Results    []TestRun     `json:"results"`
}
