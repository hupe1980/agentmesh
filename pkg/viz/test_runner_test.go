package viz

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTestRunner_NewTestRunner(t *testing.T) {
	// Create a simple server
	server := &Server{}
	runner := NewTestRunner(server)

	require.NotNil(t, runner)
	require.Equal(t, server, runner.server)
	require.NotNil(t, runner.history)
	require.Empty(t, runner.history)
}

func TestTestRunner_FilterTests(t *testing.T) {
	runner := &TestRunner{}

	suite := &TestSuite{
		Tests: []TestCase{
			{Name: "test1", Tags: []string{"unit", "fast"}},
			{Name: "test2", Tags: []string{"integration"}},
			{Name: "test3", Tags: []string{"unit", "slow"}},
			{Name: "special_test", Tags: []string{"unit"}},
		},
	}

	tests := []struct {
		name     string
		req      TestRunRequest
		expected []string
	}{
		{
			name:     "no filter",
			req:      TestRunRequest{},
			expected: []string{"test1", "test2", "test3", "special_test"},
		},
		{
			name:     "filter by test names",
			req:      TestRunRequest{Tests: []string{"test1"}},
			expected: []string{"test1"},
		},
		{
			name:     "filter by multiple names",
			req:      TestRunRequest{Tests: []string{"test1", "special_test"}},
			expected: []string{"test1", "special_test"},
		},
		{
			name:     "filter by single tag",
			req:      TestRunRequest{Tags: []string{"integration"}},
			expected: []string{"test2"},
		},
		{
			name:     "filter by multiple tags (OR logic - any must match)",
			req:      TestRunRequest{Tags: []string{"unit", "fast"}},
			expected: []string{"test1", "test3", "special_test"}, // All have "unit" or "fast"
		},
		{
			name:     "filter by names and tags",
			req:      TestRunRequest{Tests: []string{"test1", "test2", "test3"}, Tags: []string{"unit"}},
			expected: []string{"test1", "test3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filtered := runner.filterTests(suite, tt.req)

			require.Len(t, filtered, len(tt.expected))
			for i, name := range tt.expected {
				require.Equal(t, name, filtered[i].Name)
			}
		})
	}
}

func TestTestRunner_CompareOutputs_Identical(t *testing.T) {
	runner := &TestRunner{}

	expected := map[string]any{
		"result":  "success",
		"count":   42,
		"enabled": true,
		"items":   []any{"a", "b", "c"},
		"nested": map[string]any{
			"key": "value",
		},
	}

	actual := map[string]any{
		"result":  "success",
		"count":   42,
		"enabled": true,
		"items":   []any{"a", "b", "c"},
		"nested": map[string]any{
			"key": "value",
		},
	}

	diffs := runner.compareOutputs(expected, actual)
	require.Empty(t, diffs)
}

func TestTestRunner_CompareOutputs_Differences(t *testing.T) {
	runner := &TestRunner{}

	expected := map[string]any{
		"result":        "success",
		"count":         42,
		"only_expected": "present",
	}

	actual := map[string]any{
		"result":      "failure",
		"count":       43,
		"only_actual": "present",
	}

	diffs := runner.compareOutputs(expected, actual)
	require.NotEmpty(t, diffs)

	// Should have differences: result modified, count modified, only_expected removed, only_actual added
	require.GreaterOrEqual(t, len(diffs), 3)
}

func TestTestRunner_CompareOutputs_TypeMismatch(t *testing.T) {
	runner := &TestRunner{}

	expected := map[string]any{
		"value": "string",
	}

	actual := map[string]any{
		"value": 42,
	}

	diffs := runner.compareOutputs(expected, actual)
	require.Len(t, diffs, 1)
	require.Equal(t, "value", diffs[0].Path)
	require.Equal(t, "modified", diffs[0].Type) // Type is "modified", not "type_mismatch"
}

func TestTestRunner_CalculateStatistics_Empty(t *testing.T) {
	runner := &TestRunner{}

	history := &TestHistory{
		Runs: []TestRunSummary{},
	}

	runner.calculateStatistics(history)

	require.Equal(t, 0.0, history.PassRate)
	require.Equal(t, time.Duration(0), history.AvgDuration)
	require.False(t, history.Flaky)
	require.Equal(t, "", history.Trend) // Empty when no runs
}

func TestTestRunner_CalculateStatistics_AllPassed(t *testing.T) {
	runner := &TestRunner{}

	history := &TestHistory{
		Runs: []TestRunSummary{
			{Status: TestStatusPassed, Duration: 100 * time.Millisecond},
			{Status: TestStatusPassed, Duration: 200 * time.Millisecond},
			{Status: TestStatusPassed, Duration: 300 * time.Millisecond},
		},
	}

	runner.calculateStatistics(history)

	require.Equal(t, 1.0, history.PassRate) // 1.0 = 100%
	require.Equal(t, 200*time.Millisecond, history.AvgDuration)
	require.False(t, history.Flaky)
	require.Equal(t, "stable", history.Trend)
}

func TestTestRunner_CalculateStatistics_Mixed(t *testing.T) {
	runner := &TestRunner{}

	history := &TestHistory{
		Runs: []TestRunSummary{
			{Status: TestStatusPassed, Duration: 100 * time.Millisecond},
			{Status: TestStatusFailed, Duration: 150 * time.Millisecond},
			{Status: TestStatusPassed, Duration: 200 * time.Millisecond},
			{Status: TestStatusPassed, Duration: 250 * time.Millisecond},
		},
	}

	runner.calculateStatistics(history)

	require.Equal(t, 0.75, history.PassRate) // 0.75 = 75%
	require.Equal(t, 175*time.Millisecond, history.AvgDuration)
	require.True(t, history.Flaky) // 2 status changes with >=3 runs = flaky
}

func TestTestRunner_CalculateStatistics_Flaky(t *testing.T) {
	runner := &TestRunner{}

	// Flaky: at least 3 runs with alternating pass/fail
	history := &TestHistory{
		Runs: []TestRunSummary{
			{Status: TestStatusPassed},
			{Status: TestStatusFailed},
			{Status: TestStatusPassed},
			{Status: TestStatusFailed},
			{Status: TestStatusPassed},
		},
	}

	runner.calculateStatistics(history)

	require.True(t, history.Flaky)
	require.Equal(t, 0.6, history.PassRate) // 0.6 = 60%
}

func TestTestRunner_CalculateStatistics_Trends(t *testing.T) {
	runner := &TestRunner{}

	tests := []struct {
		name     string
		runs     []TestStatus
		expected string
	}{
		{
			name: "improving trend (4/5 passed)",
			runs: []TestStatus{
				TestStatusFailed, TestStatusPassed, TestStatusPassed, TestStatusPassed, TestStatusPassed,
			},
			expected: "improving",
		},
		{
			name: "degrading trend (1/5 passed)",
			runs: []TestStatus{
				TestStatusFailed, TestStatusFailed, TestStatusFailed, TestStatusFailed, TestStatusPassed,
			},
			expected: "degrading",
		},
		{
			name: "stable trend (all passed)",
			runs: []TestStatus{
				TestStatusPassed, TestStatusPassed, TestStatusPassed, TestStatusPassed, TestStatusPassed,
			},
			expected: "stable",
		},
		{
			name: "stable trend (3/5 passed)",
			runs: []TestStatus{
				TestStatusPassed, TestStatusFailed, TestStatusPassed, TestStatusFailed, TestStatusPassed,
			},
			expected: "stable",
		},
		{
			name:     "less than 5 runs",
			runs:     []TestStatus{TestStatusPassed, TestStatusFailed},
			expected: "stable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			history := &TestHistory{
				Runs: make([]TestRunSummary, len(tt.runs)),
			}
			for i, status := range tt.runs {
				history.Runs[i] = TestRunSummary{Status: status}
			}

			runner.calculateStatistics(history)

			require.Equal(t, tt.expected, history.Trend)
		})
	}
}

func TestTestRunner_UpdateHistory(t *testing.T) {
	runner := &TestRunner{
		history: make(map[string]*TestHistory),
	}

	suiteID := "suite1"
	testName := "test1"

	result := TestRun{
		ID:        "run1",
		SuiteID:   suiteID,
		TestName:  testName,
		Status:    TestStatusPassed,
		StartTime: time.Now(),
		EndTime:   time.Now().Add(100 * time.Millisecond),
	}

	// First update
	runner.updateHistory(suiteID, testName, result)

	key := suiteID + ":" + testName
	require.Contains(t, runner.history, key)
	require.Len(t, runner.history[key].Runs, 1)
	require.Equal(t, TestStatusPassed, runner.history[key].Runs[0].Status)

	// Second update
	result2 := TestRun{
		ID:        "run2",
		SuiteID:   suiteID,
		TestName:  testName,
		Status:    TestStatusFailed,
		StartTime: time.Now(),
		EndTime:   time.Now().Add(200 * time.Millisecond),
	}

	runner.updateHistory(suiteID, testName, result2)

	require.Len(t, runner.history[key].Runs, 2)
	require.Equal(t, TestStatusPassed, runner.history[key].Runs[0].Status) // Oldest first
	require.Equal(t, TestStatusFailed, runner.history[key].Runs[1].Status) // Most recent last

	// Verify statistics were calculated
	require.Equal(t, 0.5, runner.history[key].PassRate) // 0.5 = 50%
}

func TestTestRunner_GetHistory(t *testing.T) {
	runner := &TestRunner{
		history: map[string]*TestHistory{
			"suite1:test1": {
				Runs: []TestRunSummary{
					{ID: "run1", Status: TestStatusPassed},
				},
			},
		},
	}

	history := runner.GetHistory("suite1", "test1")
	require.NotNil(t, history)
	require.Len(t, history.Runs, 1)

	// Verify it's a copy
	history.Runs[0].Status = TestStatusFailed
	require.Equal(t, TestStatusPassed, runner.history["suite1:test1"].Runs[0].Status)

	// Non-existent history
	history = runner.GetHistory("suite2", "test2")
	require.Nil(t, history)
}

func TestTestRunner_CompareRuns(t *testing.T) {
	runner := &TestRunner{}

	baseStart := time.Now()
	baseRun := &TestRun{
		ID:         "run1",
		Status:     TestStatusPassed,
		StartTime:  baseStart,
		EndTime:    baseStart.Add(100 * time.Millisecond),
		Duration:   100 * time.Millisecond,
		TokensUsed: 1000,
		CostUSD:    0.01,
		Actual: map[string]any{
			"result": "success",
			"count":  10,
		},
	}

	compareStart := time.Now()
	compareRun := &TestRun{
		ID:         "run2",
		Status:     TestStatusPassed,
		StartTime:  compareStart,
		EndTime:    compareStart.Add(200 * time.Millisecond),
		Duration:   200 * time.Millisecond,
		TokensUsed: 1500,
		CostUSD:    0.015,
		Actual: map[string]any{
			"result": "success",
			"count":  15,
		},
	}

	result := runner.CompareRuns(baseRun, compareRun)

	require.NotNil(t, result)

	// Duration delta (200ms - 100ms = 100ms)
	require.Equal(t, 100*time.Millisecond, result.DurationDelta)

	// Metrics delta
	require.Equal(t, 500, result.TokensDelta)
	require.InDelta(t, 0.005, result.CostDelta, 0.0001)

	// Output differences (count changed from 10 to 15)
	require.NotEmpty(t, result.OutputDiffs)
	require.GreaterOrEqual(t, len(result.OutputDiffs), 1)
}

func TestTestRunner_SerializeDeserialize(t *testing.T) {
	runner := &TestRunner{}

	original := &TestRun{
		ID:       "run1",
		SuiteID:  "suite1",
		TestName: "test1",
		Status:   TestStatusPassed,
		Input: map[string]any{
			"query": "test query",
			"count": "5", // Use string to avoid JSON int->float64 conversion
		},
		Expected: map[string]any{
			"result": "success",
		},
		Actual: map[string]any{
			"result": "success",
		},
		Diffs:      []Diff{},
		StartTime:  time.Now().Truncate(time.Second),
		EndTime:    time.Now().Add(time.Minute).Truncate(time.Second),
		TokensUsed: 100,
		CostUSD:    0.001,
	}

	// Serialize
	data, err := runner.SerializeTestRun(original)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	// Deserialize
	deserialized, err := runner.DeserializeTestRun(data)
	require.NoError(t, err)
	require.NotNil(t, deserialized)

	// Verify basic fields
	require.Equal(t, original.ID, deserialized.ID)
	require.Equal(t, original.SuiteID, deserialized.SuiteID)
	require.Equal(t, original.TestName, deserialized.TestName)
	require.Equal(t, original.Status, deserialized.Status)

	// Verify map fields exist and have correct length
	require.Len(t, deserialized.Input, len(original.Input))
	require.Len(t, deserialized.Expected, len(original.Expected))
	require.Len(t, deserialized.Actual, len(original.Actual))

	// Verify timestamps
	require.Equal(t, original.StartTime.Unix(), deserialized.StartTime.Unix())
	require.Equal(t, original.EndTime.Unix(), deserialized.EndTime.Unix())

	// Verify metrics
	require.Equal(t, original.TokensUsed, deserialized.TokensUsed)
	require.InDelta(t, original.CostUSD, deserialized.CostUSD, 0.0001)
}

func TestTestRunner_ValuesEqual(t *testing.T) {
	runner := &TestRunner{}

	tests := []struct {
		name     string
		a        any
		b        any
		expected bool
	}{
		{"identical strings", "hello", "hello", true},
		{"different strings", "hello", "world", false},
		{"identical ints", 42, 42, true},
		{"different ints", 42, 43, false},
		{"identical bools", true, true, true},
		{"different bools", true, false, false},
		{"identical floats", 3.14, 3.14, true},
		{"different floats", 3.14, 3.15, false},
		{"nil values", nil, nil, true},
		{"nil vs non-nil", nil, "value", false},
		{"identical arrays", []any{1, 2, 3}, []any{1, 2, 3}, true},
		{"different arrays", []any{1, 2, 3}, []any{1, 2, 4}, false},
		{"different array lengths", []any{1, 2}, []any{1, 2, 3}, false},
		{"identical maps", map[string]any{"a": 1}, map[string]any{"a": 1}, true},
		{"different maps", map[string]any{"a": 1}, map[string]any{"a": 2}, false},
		{"different map keys", map[string]any{"a": 1}, map[string]any{"b": 1}, false},
		{"type mismatch", 42, "42", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := runner.valuesEqual(tt.a, tt.b)
			require.Equal(t, tt.expected, result)
		})
	}
}
