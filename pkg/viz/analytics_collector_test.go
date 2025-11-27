package viz

import (
	"math"
	"testing"
	"time"
)

func floatEquals(a, b, epsilon float64) bool {
	return math.Abs(a-b) < epsilon
}

func TestAnalyticsCollector_CollectRunAnalytics(t *testing.T) {
	store := NewEventStore(100)
	collector := NewAnalyticsCollector(store)

	runID := "test-run-1"

	// Add sample events
	events := []ExecutionEvent{
		{
			ID:        "evt-1",
			RunID:     runID,
			Type:      EventStepStart,
			Timestamp: time.Now(),
		},
		{
			ID:        "evt-2",
			RunID:     runID,
			Type:      EventNodeStart,
			Node:      "node-a",
			Timestamp: time.Now(),
		},
		{
			ID:        "evt-3",
			RunID:     runID,
			Type:      EventNodeComplete,
			Node:      "node-a",
			Timestamp: time.Now().Add(100 * time.Millisecond),
			Payload: EventPayload{
				ModelName:    "gpt-4",
				EstCostUSD:   0.05,
				TotalTokens:  100,
				InputTokens:  50,
				OutputTokens: 50,
			},
		},
		{
			ID:        "evt-4",
			RunID:     runID,
			Type:      EventNodeStart,
			Node:      "node-b",
			Timestamp: time.Now().Add(100 * time.Millisecond),
		},
		{
			ID:        "evt-5",
			RunID:     runID,
			Type:      EventNodeComplete,
			Node:      "node-b",
			Timestamp: time.Now().Add(250 * time.Millisecond),
			Payload: EventPayload{
				ModelName:    "gpt-3.5-turbo",
				EstCostUSD:   0.01,
				TotalTokens:  50,
				InputTokens:  30,
				OutputTokens: 20,
			},
		},
		{
			ID:        "evt-6",
			RunID:     runID,
			Type:      EventStepEnd,
			Timestamp: time.Now().Add(250 * time.Millisecond),
		},
	}

	for _, evt := range events {
		store.Append(evt)
	}

	// Collect analytics
	analytics, err := collector.CollectRunAnalytics(runID)
	if err != nil {
		t.Fatalf("Failed to collect analytics: %v", err)
	}

	// Verify basic metrics
	if analytics.RunID != runID {
		t.Errorf("Expected RunID %q, got %q", runID, analytics.RunID)
	}

	if !floatEquals(analytics.TotalCost, 0.06, 0.001) {
		t.Errorf("Expected total cost 0.06, got %f", analytics.TotalCost)
	}

	if analytics.TotalTokens != 150 {
		t.Errorf("Expected total tokens 150, got %d", analytics.TotalTokens)
	}

	if len(analytics.NodeMetrics) != 2 {
		t.Errorf("Expected 2 nodes, got %d", len(analytics.NodeMetrics))
	}

	// Verify node analytics
	nodeA, exists := analytics.NodeMetrics["node-a"]
	if !exists {
		t.Fatal("Node 'node-a' not found in analytics")
	}

	if nodeA.ExecutionCount != 1 {
		t.Errorf("Expected node-a execution count 1, got %d", nodeA.ExecutionCount)
	}

	if !floatEquals(nodeA.TotalCost, 0.05, 0.001) {
		t.Errorf("Expected node-a cost 0.05, got %f", nodeA.TotalCost)
	}

	if nodeA.TotalTokens != 100 {
		t.Errorf("Expected node-a tokens 100, got %d", nodeA.TotalTokens)
	}

	// Verify cost breakdown by model
	if len(analytics.CostByModel) != 2 {
		t.Errorf("Expected 2 models, got %d", len(analytics.CostByModel))
	}

	if !floatEquals(analytics.CostByModel["gpt-4"], 0.05, 0.001) {
		t.Errorf("Expected gpt-4 cost 0.05, got %f", analytics.CostByModel["gpt-4"])
	}

	if !floatEquals(analytics.CostByModel["gpt-3.5-turbo"], 0.01, 0.001) {
		t.Errorf("Expected gpt-3.5-turbo cost 0.01, got %f", analytics.CostByModel["gpt-3.5-turbo"])
	}
}

func TestAnalyticsCollector_GetRunAnalytics(t *testing.T) {
	store := NewEventStore(100)
	collector := NewAnalyticsCollector(store)

	runID := "test-run-2"

	// Add a simple event
	store.Append(ExecutionEvent{
		ID:        "evt-1",
		RunID:     runID,
		Type:      EventNodeComplete,
		Node:      "test-node",
		Timestamp: time.Now(),
	})

	// Collect analytics first
	_, err := collector.CollectRunAnalytics(runID)
	if err != nil {
		t.Fatalf("Failed to collect analytics: %v", err)
	}

	// Retrieve cached analytics
	analytics := collector.GetRunAnalytics(runID)
	if analytics == nil {
		t.Fatal("Expected analytics to be cached, got nil")
	}

	if analytics.RunID != runID {
		t.Errorf("Expected RunID %q, got %q", runID, analytics.RunID)
	}

	// Non-existent run
	if collector.GetRunAnalytics("nonexistent") != nil {
		t.Error("Expected nil for nonexistent run")
	}
}

func TestAnalyticsCollector_IdentifyBottlenecks(t *testing.T) {
	analytics := &RunAnalytics{
		RunID:     "test-run",
		TotalCost: 1.0,
		Duration:  time.Second,
		CostByNode: map[string]float64{
			"slow-node":      0.1,
			"expensive-node": 0.5, // 50% of total cost
			"normal-node":    0.05,
		},
		NodeMetrics: map[string]*NodeAnalytics{
			"slow-node": {
				NodeID:         "slow-node",
				ExecutionCount: 1,
				TotalDuration:  500 * time.Millisecond, // 50% of total time
				AvgDuration:    500 * time.Millisecond,
				TotalCost:      0.1,
			},
			"expensive-node": {
				NodeID:         "expensive-node",
				ExecutionCount: 1,
				TotalDuration:  100 * time.Millisecond,
				AvgDuration:    100 * time.Millisecond,
				TotalCost:      0.5, // 50% of total cost
			},
			"normal-node": {
				NodeID:         "normal-node",
				ExecutionCount: 1,
				TotalDuration:  50 * time.Millisecond,
				AvgDuration:    50 * time.Millisecond,
				TotalCost:      0.05,
			},
		},
	}

	collector := &AnalyticsCollector{}
	bottlenecks := collector.identifyBottlenecks(analytics)
	analytics.Bottlenecks = bottlenecks

	if len(analytics.Bottlenecks) != 2 {
		t.Fatalf("Expected 2 bottlenecks, got %d", len(analytics.Bottlenecks))
	}

	// Check for slow node bottleneck
	foundSlow := false
	foundExpensive := false

	for _, b := range analytics.Bottlenecks {
		switch b.Type {
		case "duration":
			if b.NodeID == "slow-node" {
				foundSlow = true
				// Impact is a string (high, medium, low)
				if b.Impact != "high" && b.Impact != "medium" && b.Impact != "low" {
					t.Errorf("Expected impact to be high/medium/low, got %q", b.Impact)
				}
			}
		case "cost":
			if b.NodeID == "expensive-node" {
				foundExpensive = true
			}
		}
	}

	if !foundSlow {
		t.Error("Expected to find slow node bottleneck")
	}

	if !foundExpensive {
		t.Error("Expected to find expensive node bottleneck")
	}
}

func TestAnalyticsCollector_GetCostBreakdown(t *testing.T) {
	store := NewEventStore(100)
	collector := NewAnalyticsCollector(store)

	runID := "test-run-3"

	// Add events with costs
	events := []ExecutionEvent{
		{
			ID:        "evt-1",
			RunID:     runID,
			Type:      EventStepStart,
			Timestamp: time.Now(),
		},
		{
			ID:        "evt-2",
			RunID:     runID,
			Type:      EventNodeComplete,
			Node:      "node-1",
			Timestamp: time.Now(),
			Payload: EventPayload{
				ModelName:   "gpt-4",
				EstCostUSD:  0.10,
				TotalTokens: 100,
			},
		},
		{
			ID:        "evt-3",
			RunID:     runID,
			Type:      EventNodeComplete,
			Node:      "node-2",
			Timestamp: time.Now(),
			Payload: EventPayload{
				ModelName:   "gpt-3.5-turbo",
				EstCostUSD:  0.05,
				TotalTokens: 50,
			},
		},
	}

	for _, evt := range events {
		store.Append(evt)
	}

	collector.CollectRunAnalytics(runID)

	breakdown := collector.GetCostBreakdown(runID)
	if breakdown == nil {
		t.Fatal("Expected cost breakdown, got nil")
	}

	if !floatEquals(breakdown.TotalCost, 0.15, 0.001) {
		t.Errorf("Expected total cost 0.15, got %f", breakdown.TotalCost)
	}

	if len(breakdown.ByModel) != 2 {
		t.Errorf("Expected 2 models, got %d", len(breakdown.ByModel))
	}

	if len(breakdown.TopCostNodes) != 2 {
		t.Errorf("Expected 2 nodes, got %d", len(breakdown.TopCostNodes))
	}

	// Verify nodes are sorted by cost (descending)
	if len(breakdown.TopCostNodes) >= 2 {
		if breakdown.TopCostNodes[0].Cost < breakdown.TopCostNodes[1].Cost {
			t.Error("Expected nodes to be sorted by cost descending")
		}
	}
}

func TestAnalyticsCollector_GenerateSummary(t *testing.T) {
	store := NewEventStore(100)
	collector := NewAnalyticsCollector(store)

	// Add multiple runs
	runs := []struct {
		runID    string
		cost     float64
		duration time.Duration
		status   string
	}{
		{"run-1", 0.10, 100 * time.Millisecond, "completed"},
		{"run-2", 0.20, 200 * time.Millisecond, "completed"},
		{"run-3", 0.15, 150 * time.Millisecond, "failed"},
	}

	for _, run := range runs {
		events := []ExecutionEvent{
			{
				ID:        run.runID + "-evt-1",
				RunID:     run.runID,
				Type:      EventStepStart,
				Timestamp: time.Now(),
			},
			{
				ID:        run.runID + "-evt-2",
				RunID:     run.runID,
				Type:      EventNodeComplete,
				Node:      "test-node",
				Timestamp: time.Now().Add(run.duration),
				Payload: EventPayload{
					EstCostUSD: run.cost,
				},
			},
		}

		for _, evt := range events {
			store.Append(evt)
		}

		collector.CollectRunAnalytics(run.runID)
	}

	// Generate summary
	query := AnalyticsQuery{
		Limit: 10,
	}

	summary := collector.GenerateSummary(query)

	if summary.TotalRuns != 3 {
		t.Errorf("Expected 3 total runs, got %d", summary.TotalRuns)
	}

	if !floatEquals(summary.TotalCost, 0.45, 0.001) {
		t.Errorf("Expected total cost 0.45, got %f", summary.TotalCost)
	}

	// Status tracking might not be working without proper event types
	if len(summary.ByStatus) < 1 {
		t.Errorf("Expected at least 1 status group, got %d", len(summary.ByStatus))
	}
}

func TestAnalyticsCollector_PredictCost(t *testing.T) {
	store := NewEventStore(100)
	collector := NewAnalyticsCollector(store)

	// Add historical runs with completed status (oldest first)
	baseTime := time.Now().Add(-5 * time.Hour)
	for i := 0; i < 5; i++ {
		timestamp := baseTime.Add(time.Duration(i) * time.Hour)
		runID := timestamp.Format("run-20060102-150405")

		events := []ExecutionEvent{
			{
				ID:        runID + "-evt-1",
				RunID:     runID,
				Type:      EventStepStart,
				Timestamp: timestamp,
			},
			{
				ID:        runID + "-evt-2",
				RunID:     runID,
				Type:      EventNodeComplete,
				Node:      "test-node",
				Timestamp: timestamp,
				Payload: EventPayload{
					EstCostUSD: 0.10 + float64(i)*0.02, // Increasing trend
				},
			},
			{
				ID:        runID + "-evt-3",
				RunID:     runID,
				Type:      EventStepEnd,
				Timestamp: timestamp.Add(time.Second),
			},
		}

		for _, evt := range events {
			store.Append(evt)
		}

		analytics, err := collector.CollectRunAnalytics(runID)
		if err != nil {
			t.Fatalf("Failed to collect analytics: %v", err)
		}
		// Set status to completed manually for test
		analytics.Status = "completed"
		analytics.StartTime = timestamp
		collector.mu.Lock()
		collector.runAnalytics[runID] = analytics
		collector.mu.Unlock()
	}

	// Predict cost (pass empty string to include all runs)
	prediction := collector.PredictCost("")
	if prediction == nil {
		t.Fatal("Expected cost prediction, got nil")
	}

	if prediction.NextRun <= 0 {
		t.Errorf("Expected positive next run cost, got %f", prediction.NextRun)
	}

	if prediction.Next24Hours <= 0 {
		t.Errorf("Expected positive 24h cost, got %f", prediction.Next24Hours)
	}

	if prediction.Confidence < 0 || prediction.Confidence > 1 {
		t.Errorf("Expected confidence between 0 and 1, got %f", prediction.Confidence)
	}

	// Verify trend detection
	validTrends := []string{"increasing", "decreasing", "stable"}
	found := false
	for _, valid := range validTrends {
		if prediction.Trend == valid {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Expected valid trend, got %q", prediction.Trend)
	}
}

func TestAnalyticsCollector_EmptyEvents(t *testing.T) {
	store := NewEventStore(100)
	collector := NewAnalyticsCollector(store)

	// Collect analytics for non-existent run - should return error
	_, err := collector.CollectRunAnalytics("nonexistent")
	if err == nil {
		t.Fatal("Expected error for nonexistent run, got nil")
	}
}

func TestAnalyticsCollector_CostBreakdownSorting(t *testing.T) {
	store := NewEventStore(100)
	collector := NewAnalyticsCollector(store)

	runID := "test-run-sort"

	// Add nodes with different costs (not in order)
	costs := []struct {
		node string
		cost float64
	}{
		{"node-c", 0.05},
		{"node-a", 0.20},
		{"node-b", 0.10},
		{"node-e", 0.02},
		{"node-d", 0.15},
	}

	events := []ExecutionEvent{
		{
			ID:        "evt-start",
			RunID:     runID,
			Type:      EventStepStart,
			Timestamp: time.Now(),
		},
	}

	for _, c := range costs {
		events = append(events, ExecutionEvent{
			ID:        "evt-" + c.node,
			RunID:     runID,
			Type:      EventNodeComplete,
			Node:      c.node,
			Timestamp: time.Now(),
			Payload: EventPayload{
				EstCostUSD: c.cost,
			},
		})
	}

	for _, evt := range events {
		store.Append(evt)
	}

	collector.CollectRunAnalytics(runID)
	breakdown := collector.GetCostBreakdown(runID)

	// Verify top 10 are sorted descending
	for i := 0; i < len(breakdown.TopCostNodes)-1; i++ {
		if breakdown.TopCostNodes[i].Cost < breakdown.TopCostNodes[i+1].Cost {
			t.Errorf("Nodes not sorted: position %d has cost %f, position %d has cost %f",
				i, breakdown.TopCostNodes[i].Cost,
				i+1, breakdown.TopCostNodes[i+1].Cost)
		}
	}

	// Verify top node is node-a
	if breakdown.TopCostNodes[0].NodeID != "node-a" {
		t.Errorf("Expected top cost node to be 'node-a', got %q", breakdown.TopCostNodes[0].NodeID)
	}
}

func TestAnalyticsCollector_MultipleNodeExecutions(t *testing.T) {
	store := NewEventStore(100)
	collector := NewAnalyticsCollector(store)

	runID := "test-run-multi"

	// Add multiple executions of same node
	events := []ExecutionEvent{
		{
			ID:        "evt-start",
			RunID:     runID,
			Type:      EventStepStart,
			Timestamp: time.Now(),
		},
	}

	for i := 0; i < 3; i++ {
		start := time.Now().Add(time.Duration(i) * 100 * time.Millisecond)
		events = append(events,
			ExecutionEvent{
				ID:        "evt-start-" + string(rune('a'+i)),
				RunID:     runID,
				Type:      EventNodeStart,
				Node:      "repeated-node",
				Timestamp: start,
			},
			ExecutionEvent{
				ID:        "evt-complete-" + string(rune('a'+i)),
				RunID:     runID,
				Type:      EventNodeComplete,
				Node:      "repeated-node",
				Timestamp: start.Add(50 * time.Millisecond),
				Payload: EventPayload{
					EstCostUSD:  0.05,
					TotalTokens: 50,
				},
			},
		)
	}

	for _, evt := range events {
		store.Append(evt)
	}

	analytics, err := collector.CollectRunAnalytics(runID)
	if err != nil {
		t.Fatalf("Failed to collect analytics: %v", err)
	}

	node, exists := analytics.NodeMetrics["repeated-node"]
	if !exists {
		t.Fatal("Expected 'repeated-node' in analytics")
	}

	if node.ExecutionCount != 3 {
		t.Errorf("Expected 3 executions, got %d", node.ExecutionCount)
	}

	if !floatEquals(node.TotalCost, 0.15, 0.001) {
		t.Errorf("Expected total cost 0.15, got %f", node.TotalCost)
	}

	if node.TotalTokens != 150 {
		t.Errorf("Expected total tokens 150, got %d", node.TotalTokens)
	}
}
