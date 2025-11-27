package viz

import (
	"sort"
	"sync"
	"time"
)

// AnalyticsCollector collects and aggregates execution analytics.
type AnalyticsCollector struct {
	mu           sync.RWMutex
	runAnalytics map[string]*RunAnalytics // runID -> analytics
	eventStore   *EventStore
}

// NewAnalyticsCollector creates a new analytics collector.
func NewAnalyticsCollector(eventStore *EventStore) *AnalyticsCollector {
	return &AnalyticsCollector{
		runAnalytics: make(map[string]*RunAnalytics),
		eventStore:   eventStore,
	}
}

// CollectRunAnalytics analyzes a completed run and generates analytics.
func (ac *AnalyticsCollector) CollectRunAnalytics(runID string) (*RunAnalytics, error) {
	// Get all events for the run
	events, err := ac.eventStore.GetEvents(runID, 0)
	if err != nil {
		return nil, err
	}

	if len(events) == 0 {
		return nil, nil
	}

	analytics := &RunAnalytics{
		RunID:         runID,
		CostByModel:   make(map[string]float64),
		CostByNode:    make(map[string]float64),
		TokensByModel: make(map[string]int),
		TokensByNode:  make(map[string]int),
		NodeMetrics:   make(map[string]*NodeAnalytics),
		Bottlenecks:   make([]Bottleneck, 0),
		CriticalPath:  make([]string, 0),
	}

	// Track node execution times
	nodeStartTimes := make(map[string]time.Time)
	nodeExecutions := make(map[string][]time.Duration)

	// Process events
	ac.processEvents(events, analytics, nodeStartTimes, nodeExecutions)

	// Calculate duration
	if !analytics.EndTime.IsZero() && !analytics.StartTime.IsZero() {
		analytics.Duration = analytics.EndTime.Sub(analytics.StartTime)
	}

	// Generate node metrics
	for nodeID, durations := range nodeExecutions {
		metrics := &NodeAnalytics{
			NodeID:         nodeID,
			ExecutionCount: len(durations),
			TotalCost:      analytics.CostByNode[nodeID],
			TotalTokens:    analytics.TokensByNode[nodeID],
		}

		// Calculate duration statistics
		if len(durations) > 0 {
			var total time.Duration
			metrics.MinDuration = durations[0]
			metrics.MaxDuration = durations[0]

			for _, d := range durations {
				total += d
				if d < metrics.MinDuration {
					metrics.MinDuration = d
				}
				if d > metrics.MaxDuration {
					metrics.MaxDuration = d
				}
			}

			metrics.TotalDuration = total
			metrics.AvgDuration = total / time.Duration(len(durations))
		}

		if metrics.ExecutionCount > 0 {
			metrics.AvgCost = metrics.TotalCost / float64(metrics.ExecutionCount)
			metrics.AvgTokens = metrics.TotalTokens / metrics.ExecutionCount
		}

		analytics.NodeMetrics[nodeID] = metrics
	}

	// Identify bottlenecks
	analytics.Bottlenecks = ac.identifyBottlenecks(analytics)

	// Store analytics
	ac.mu.Lock()
	ac.runAnalytics[runID] = analytics
	ac.mu.Unlock()

	return analytics, nil
}

// identifyBottlenecks finds performance bottlenecks in the execution.
func (ac *AnalyticsCollector) identifyBottlenecks(analytics *RunAnalytics) []Bottleneck {
	bottlenecks := make([]Bottleneck, 0)

	if analytics.Duration == 0 {
		return bottlenecks
	}

	// Find slow nodes (taking >20% of total time)
	threshold := float64(analytics.Duration) * 0.20
	for nodeID, metrics := range analytics.NodeMetrics {
		if float64(metrics.TotalDuration) > threshold {
			impact := "high"
			if float64(metrics.TotalDuration) < float64(analytics.Duration)*0.30 {
				impact = "medium"
			}

			bottlenecks = append(bottlenecks, Bottleneck{
				NodeID:      nodeID,
				Type:        "duration",
				Value:       metrics.TotalDuration.Seconds(),
				Impact:      impact,
				Description: "Node consumes significant execution time",
				Suggestion:  "Consider optimizing or parallelizing this node",
			})
		}
	}

	// Find expensive nodes (costing >25% of total cost)
	if analytics.TotalCost > 0 {
		costThreshold := analytics.TotalCost * 0.25
		for nodeID, cost := range analytics.CostByNode {
			if cost > costThreshold {
				percentage := (cost / analytics.TotalCost) * 100

				bottlenecks = append(bottlenecks, Bottleneck{
					NodeID:      nodeID,
					Type:        "cost",
					Value:       cost,
					Impact:      "high",
					Description: "Node accounts for high cost",
					Suggestion:  "Review model selection or prompt optimization",
				})

				_ = percentage // For potential future use
			}
		}
	}

	return bottlenecks
}

// GetRunAnalytics retrieves analytics for a specific run.
func (ac *AnalyticsCollector) GetRunAnalytics(runID string) *RunAnalytics {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	return ac.runAnalytics[runID]
}

// GetCostBreakdown generates a detailed cost breakdown.
func (ac *AnalyticsCollector) GetCostBreakdown(runID string) *CostBreakdown {
	analytics := ac.GetRunAnalytics(runID)
	if analytics == nil {
		return nil
	}

	breakdown := &CostBreakdown{
		TotalCost:    analytics.TotalCost,
		ByModel:      make(map[string]*ModelCost),
		ByNode:       analytics.CostByNode,
		TopCostNodes: make([]NodeCostSummary, 0),
	}

	// Build model costs
	for model, cost := range analytics.CostByModel {
		breakdown.ByModel[model] = &ModelCost{
			Model:       model,
			TotalCost:   cost,
			TotalTokens: analytics.TokensByModel[model],
		}
	}

	// Build top cost nodes
	type nodeCost struct {
		nodeID string
		cost   float64
	}
	nodeCosts := make([]nodeCost, 0, len(analytics.CostByNode))
	for nodeID, cost := range analytics.CostByNode {
		nodeCosts = append(nodeCosts, nodeCost{nodeID, cost})
	}

	// Sort by cost descending
	sort.Slice(nodeCosts, func(i, j int) bool {
		return nodeCosts[i].cost > nodeCosts[j].cost
	})

	// Take top 10
	limit := 10
	if len(nodeCosts) < limit {
		limit = len(nodeCosts)
	}

	for i := 0; i < limit; i++ {
		nc := nodeCosts[i]
		percentage := 0.0
		if analytics.TotalCost > 0 {
			percentage = (nc.cost / analytics.TotalCost) * 100
		}

		breakdown.TopCostNodes = append(breakdown.TopCostNodes, NodeCostSummary{
			NodeID:     nc.nodeID,
			Cost:       nc.cost,
			Tokens:     analytics.TokensByNode[nc.nodeID],
			Percentage: percentage,
		})
	}

	return breakdown
}

// GenerateSummary creates an aggregated summary across multiple runs.
//
//nolint:gocyclo // Function is primarily aggregation and sorting logic
func (ac *AnalyticsCollector) GenerateSummary(query AnalyticsQuery) *AnalyticsSummary {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	summary := &AnalyticsSummary{
		ByGraph:       make(map[string]*GraphSummary),
		ByStatus:      make(map[string]int),
		MostExpensive: make([]RunCostSummary, 0),
		Fastest:       make([]RunDurationSummary, 0),
		Slowest:       make([]RunDurationSummary, 0),
	}

	// Set time range
	if query.StartTime != nil {
		summary.TimeRange.Start = *query.StartTime
	}
	if query.EndTime != nil {
		summary.TimeRange.End = *query.EndTime
	}

	// Collect runs matching query
	var totalDuration time.Duration
	costSummaries := make([]RunCostSummary, 0)
	durationSummaries := make([]RunDurationSummary, 0)

	for runID, analytics := range ac.runAnalytics {
		// Apply filters
		if query.StartTime != nil && analytics.StartTime.Before(*query.StartTime) {
			continue
		}
		if query.EndTime != nil && analytics.EndTime.After(*query.EndTime) {
			continue
		}
		if query.GraphID != "" && analytics.GraphID != query.GraphID {
			continue
		}
		if query.Status != "" && analytics.Status != query.Status {
			continue
		}

		summary.TotalRuns++
		summary.TotalCost += analytics.TotalCost
		summary.TotalTokens += analytics.TotalTokens
		totalDuration += analytics.Duration

		// Track by status
		summary.ByStatus[analytics.Status]++

		// Track by graph
		if _, exists := summary.ByGraph[analytics.GraphID]; !exists {
			summary.ByGraph[analytics.GraphID] = &GraphSummary{
				GraphID: analytics.GraphID,
			}
		}
		gs := summary.ByGraph[analytics.GraphID]
		gs.RunCount++
		gs.TotalCost += analytics.TotalCost
		gs.TotalTokens += analytics.TotalTokens
		gs.AvgDuration = (gs.AvgDuration*time.Duration(gs.RunCount-1) + analytics.Duration) / time.Duration(gs.RunCount)
		if analytics.Status == "completed" {
			gs.SuccessRate = float64(gs.RunCount) / float64(summary.TotalRuns)
		}

		// Collect for top lists
		costSummaries = append(costSummaries, RunCostSummary{
			RunID:     runID,
			GraphID:   analytics.GraphID,
			Cost:      analytics.TotalCost,
			Tokens:    analytics.TotalTokens,
			Timestamp: analytics.StartTime,
		})

		durationSummaries = append(durationSummaries, RunDurationSummary{
			RunID:     runID,
			GraphID:   analytics.GraphID,
			Duration:  analytics.Duration,
			Timestamp: analytics.StartTime,
		})
	}

	// Calculate averages
	if summary.TotalRuns > 0 {
		summary.AvgRunDuration = totalDuration / time.Duration(summary.TotalRuns)

		for _, gs := range summary.ByGraph {
			if gs.RunCount > 0 {
				gs.AvgCost = gs.TotalCost / float64(gs.RunCount)
			}
		}
	}

	// Sort and limit top lists
	limit := 10
	if query.Limit > 0 && query.Limit < limit {
		limit = query.Limit
	}

	// Most expensive
	sort.Slice(costSummaries, func(i, j int) bool {
		return costSummaries[i].Cost > costSummaries[j].Cost
	})
	if len(costSummaries) > limit {
		summary.MostExpensive = costSummaries[:limit]
	} else {
		summary.MostExpensive = costSummaries
	}

	// Fastest
	sort.Slice(durationSummaries, func(i, j int) bool {
		return durationSummaries[i].Duration < durationSummaries[j].Duration
	})
	if len(durationSummaries) > limit {
		summary.Fastest = durationSummaries[:limit]
	} else {
		summary.Fastest = durationSummaries
	}

	// Slowest
	sort.Slice(durationSummaries, func(i, j int) bool {
		return durationSummaries[i].Duration > durationSummaries[j].Duration
	})
	if len(durationSummaries) > limit {
		summary.Slowest = durationSummaries[:limit]
	} else {
		summary.Slowest = durationSummaries
	}

	return summary
}

// PredictCost predicts future costs based on historical data.
func (ac *AnalyticsCollector) PredictCost(graphID string) *CostPrediction {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	// Collect recent runs for the graph
	costs := make([]float64, 0, len(ac.runAnalytics))
	timestamps := make([]time.Time, 0, len(ac.runAnalytics))

	for _, analytics := range ac.runAnalytics {
		if graphID != "" && analytics.GraphID != graphID {
			continue
		}
		if analytics.Status != "completed" {
			continue
		}

		costs = append(costs, analytics.TotalCost)
		timestamps = append(timestamps, analytics.StartTime)
	}

	if len(costs) == 0 {
		return nil
	}

	// Calculate average cost
	var totalCost float64
	for _, cost := range costs {
		totalCost += cost
	}
	avgCost := totalCost / float64(len(costs))

	// Simple predictions based on average
	prediction := &CostPrediction{
		NextRun:     avgCost,
		BasedOnRuns: len(costs),
		Confidence:  0.7, // Medium confidence
		Trend:       "stable",
	}

	// Estimate based on run frequency if we have timestamps
	if len(timestamps) > 1 {
		// Sort timestamps to ensure proper ordering
		sort.Slice(timestamps, func(i, j int) bool {
			return timestamps[i].Before(timestamps[j])
		})

		// Calculate average runs per day
		duration := timestamps[len(timestamps)-1].Sub(timestamps[0])
		if duration.Hours() > 0 {
			runsPerDay := float64(len(timestamps)) / duration.Hours() * 24

			prediction.Next24Hours = avgCost * runsPerDay
			prediction.Next7Days = avgCost * runsPerDay * 7
			prediction.Next30Days = avgCost * runsPerDay * 30
		}
	}

	// Determine trend if we have enough data
	if len(costs) >= 5 {
		// Compare recent half vs older half
		mid := len(costs) / 2
		var oldSum, recentSum float64

		for i := 0; i < mid; i++ {
			oldSum += costs[i]
		}
		for i := mid; i < len(costs); i++ {
			recentSum += costs[i]
		}

		oldAvg := oldSum / float64(mid)
		recentAvg := recentSum / float64(len(costs)-mid)

		if recentAvg > oldAvg*1.1 {
			prediction.Trend = "increasing"
		} else if recentAvg < oldAvg*0.9 {
			prediction.Trend = "decreasing"
		}
	}

	return prediction
}

// processEvents processes all events and updates analytics
func (ac *AnalyticsCollector) processEvents(events []ExecutionEvent, analytics *RunAnalytics, nodeStartTimes map[string]time.Time, nodeExecutions map[string][]time.Duration) {
	for i := range events {
		event := &events[i]

		// Update timestamps
		ac.updateTimestamps(event, analytics)

		// Accumulate costs and tokens
		ac.accumulateCostsAndTokens(event, analytics)

		// Track node execution
		ac.trackNodeExecution(event, nodeStartTimes, nodeExecutions)

		// Track status
		ac.updateStatus(event, analytics)

		analytics.EventCount++
	}
}

// updateTimestamps updates start and end times
func (ac *AnalyticsCollector) updateTimestamps(event *ExecutionEvent, analytics *RunAnalytics) {
	if analytics.StartTime.IsZero() || event.Timestamp.Before(analytics.StartTime) {
		analytics.StartTime = event.Timestamp
	}
	if event.Timestamp.After(analytics.EndTime) {
		analytics.EndTime = event.Timestamp
	}
}

// accumulateCostsAndTokens accumulates cost and token metrics
func (ac *AnalyticsCollector) accumulateCostsAndTokens(event *ExecutionEvent, analytics *RunAnalytics) {
	if event.Payload.EstCostUSD > 0 {
		analytics.TotalCost += event.Payload.EstCostUSD
		if event.Node != "" {
			analytics.CostByNode[event.Node] += event.Payload.EstCostUSD
		}
		if event.Payload.ModelName != "" {
			analytics.CostByModel[event.Payload.ModelName] += event.Payload.EstCostUSD
		}
	}

	if event.Payload.TotalTokens > 0 {
		analytics.TotalTokens += event.Payload.TotalTokens
		if event.Node != "" {
			analytics.TokensByNode[event.Node] += event.Payload.TotalTokens
		}
		if event.Payload.ModelName != "" {
			analytics.TokensByModel[event.Payload.ModelName] += event.Payload.TotalTokens
		}
	}
}

// trackNodeExecution tracks node execution times
func (ac *AnalyticsCollector) trackNodeExecution(event *ExecutionEvent, nodeStartTimes map[string]time.Time, nodeExecutions map[string][]time.Duration) {
	switch event.Type {
	case EventNodeStart:
		nodeStartTimes[event.Node] = event.Timestamp
	case EventNodeComplete:
		if startTime, exists := nodeStartTimes[event.Node]; exists {
			duration := event.Timestamp.Sub(startTime)
			nodeExecutions[event.Node] = append(nodeExecutions[event.Node], duration)
		}
	}
}

// updateStatus updates the run status based on events
func (ac *AnalyticsCollector) updateStatus(event *ExecutionEvent, analytics *RunAnalytics) {
	switch event.Type {
	case EventGraphComplete:
		analytics.Status = string(StatusCompleted)
	case EventGraphError:
		analytics.Status = string(StatusFailed)
	case EventInterrupt:
		analytics.Status = "canceled"
	}
}
