package viz

import (
	"time"
)

// RunAnalytics contains comprehensive analytics for a graph execution run.
type RunAnalytics struct {
	RunID     string        `json:"run_id"`
	GraphID   string        `json:"graph_id"`
	StartTime time.Time     `json:"start_time"`
	EndTime   time.Time     `json:"end_time"`
	Duration  time.Duration `json:"duration"`
	Status    string        `json:"status"` // completed, failed, canceled

	// Cost metrics
	TotalCost   float64            `json:"total_cost"`
	CostByModel map[string]float64 `json:"cost_by_model"`
	CostByNode  map[string]float64 `json:"cost_by_node"`

	// Token metrics
	TotalTokens   int            `json:"total_tokens"`
	TokensByModel map[string]int `json:"tokens_by_model"`
	TokensByNode  map[string]int `json:"tokens_by_node"`

	// Performance metrics
	NodeMetrics  map[string]*NodeAnalytics `json:"node_metrics"`
	Bottlenecks  []Bottleneck              `json:"bottlenecks"`
	CriticalPath []string                  `json:"critical_path"` // Longest execution path

	// Resource usage
	PeakMemory int64 `json:"peak_memory_bytes"`
	AvgMemory  int64 `json:"avg_memory_bytes"`
	EventCount int   `json:"event_count"`
	StateSize  int64 `json:"state_size_bytes"`

	// Metadata
	Tags     []string       `json:"tags,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// NodeAnalytics contains detailed analytics for a single node.
type NodeAnalytics struct {
	NodeID         string        `json:"node_id"`
	ExecutionCount int           `json:"execution_count"` // Times executed
	TotalDuration  time.Duration `json:"total_duration"`
	AvgDuration    time.Duration `json:"avg_duration"`
	MinDuration    time.Duration `json:"min_duration"`
	MaxDuration    time.Duration `json:"max_duration"`

	// Cost metrics
	TotalCost   float64 `json:"total_cost"`
	AvgCost     float64 `json:"avg_cost"`
	TotalTokens int     `json:"total_tokens"`
	AvgTokens   int     `json:"avg_tokens"`

	// Performance
	QueueTime     time.Duration `json:"queue_time"`     // Time waiting in queue
	ExecutionTime time.Duration `json:"execution_time"` // Actual execution

	// Status tracking
	SuccessCount int `json:"success_count"`
	ErrorCount   int `json:"error_count"`
	RetryCount   int `json:"retry_count"`

	// Resource usage
	MemoryUsage int64 `json:"memory_usage_bytes"`
}

// Bottleneck identifies performance bottlenecks in execution.
type Bottleneck struct {
	NodeID      string  `json:"node_id"`
	Type        string  `json:"type"` // duration, cost, memory, queue_time
	Value       float64 `json:"value"`
	Impact      string  `json:"impact"` // high, medium, low
	Description string  `json:"description"`
	Suggestion  string  `json:"suggestion,omitempty"`
}

// CostBreakdown provides detailed cost analysis.
type CostBreakdown struct {
	TotalCost    float64               `json:"total_cost"`
	ByModel      map[string]*ModelCost `json:"by_model"`
	ByNode       map[string]float64    `json:"by_node"`
	ByTimeRange  []*TimeRangeCost      `json:"by_time_range"`
	TopCostNodes []NodeCostSummary     `json:"top_cost_nodes"`
	Predictions  *CostPrediction       `json:"predictions,omitempty"`
}

// ModelCost tracks costs for a specific model.
type ModelCost struct {
	Model         string  `json:"model"`
	TotalCost     float64 `json:"total_cost"`
	TotalTokens   int     `json:"total_tokens"`
	RequestCount  int     `json:"request_count"`
	AvgCostPerReq float64 `json:"avg_cost_per_request"`
	InputTokens   int     `json:"input_tokens"`
	OutputTokens  int     `json:"output_tokens"`
}

// TimeRangeCost tracks costs over time ranges.
type TimeRangeCost struct {
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Cost      float64   `json:"cost"`
	Tokens    int       `json:"tokens"`
	RunCount  int       `json:"run_count"`
}

// NodeCostSummary summarizes costs for a node.
type NodeCostSummary struct {
	NodeID     string  `json:"node_id"`
	Cost       float64 `json:"cost"`
	Tokens     int     `json:"tokens"`
	Percentage float64 `json:"percentage"` // % of total cost
}

// CostPrediction forecasts future costs based on historical data.
type CostPrediction struct {
	NextRun     float64 `json:"next_run_estimate"`
	Next24Hours float64 `json:"next_24h_estimate"`
	Next7Days   float64 `json:"next_7d_estimate"`
	Next30Days  float64 `json:"next_30d_estimate"`
	Confidence  float64 `json:"confidence"` // 0.0-1.0
	BasedOnRuns int     `json:"based_on_runs"`
	Trend       string  `json:"trend"` // increasing, stable, decreasing
}

// PerformanceProfile contains execution performance metrics.
type PerformanceProfile struct {
	RunID         string                 `json:"run_id"`
	Duration      time.Duration          `json:"duration"`
	NodeTimings   map[string]*NodeTiming `json:"node_timings"`
	ExecutionPath []PathSegment          `json:"execution_path"`
	Parallelism   *ParallelismMetrics    `json:"parallelism"`
	ResourceUsage *ResourceUsage         `json:"resource_usage"`
}

// NodeTiming tracks timing details for a node.
type NodeTiming struct {
	NodeID        string                   `json:"node_id"`
	StartTime     time.Time                `json:"start_time"`
	EndTime       time.Time                `json:"end_time"`
	Duration      time.Duration            `json:"duration"`
	QueuedAt      time.Time                `json:"queued_at,omitempty"`
	QueueDuration time.Duration            `json:"queue_duration"`
	Phases        map[string]time.Duration `json:"phases,omitempty"` // init, execution, cleanup
}

// PathSegment represents a segment of the execution path.
type PathSegment struct {
	FromNode   string        `json:"from_node"`
	ToNode     string        `json:"to_node"`
	Timestamp  time.Time     `json:"timestamp"`
	Duration   time.Duration `json:"duration"`
	OnCritical bool          `json:"on_critical_path"`
}

// ParallelismMetrics tracks parallel execution efficiency.
type ParallelismMetrics struct {
	MaxConcurrent      int           `json:"max_concurrent_nodes"`
	AvgConcurrent      float64       `json:"avg_concurrent_nodes"`
	ParallelEfficiency float64       `json:"parallel_efficiency"` // 0.0-1.0
	SerialTime         time.Duration `json:"serial_time"`
	ParallelTime       time.Duration `json:"parallel_time"`
	SpeedupFactor      float64       `json:"speedup_factor"`
}

// ResourceUsage tracks resource consumption.
type ResourceUsage struct {
	PeakMemory int64          `json:"peak_memory_bytes"`
	AvgMemory  int64          `json:"avg_memory_bytes"`
	MinMemory  int64          `json:"min_memory_bytes"`
	Samples    []MemorySample `json:"samples,omitempty"`
}

// MemorySample represents a memory measurement at a point in time.
type MemorySample struct {
	Timestamp time.Time `json:"timestamp"`
	Bytes     int64     `json:"bytes"`
	NodeID    string    `json:"node_id,omitempty"`
}

// AnalyticsSummary provides aggregated analytics across multiple runs.
type AnalyticsSummary struct {
	TimeRange      TimeRange     `json:"time_range"`
	TotalRuns      int           `json:"total_runs"`
	TotalCost      float64       `json:"total_cost"`
	TotalTokens    int           `json:"total_tokens"`
	AvgRunDuration time.Duration `json:"avg_run_duration"`

	// Breakdowns
	ByGraph   map[string]*GraphSummary `json:"by_graph"`
	ByStatus  map[string]int           `json:"by_status"`
	CostTrend []TimeRangeCost          `json:"cost_trend"`

	// Top performers/consumers
	MostExpensive []RunCostSummary     `json:"most_expensive_runs"`
	Fastest       []RunDurationSummary `json:"fastest_runs"`
	Slowest       []RunDurationSummary `json:"slowest_runs"`
}

// TimeRange represents a time period.
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// GraphSummary summarizes analytics for a specific graph.
type GraphSummary struct {
	GraphID     string        `json:"graph_id"`
	RunCount    int           `json:"run_count"`
	TotalCost   float64       `json:"total_cost"`
	AvgCost     float64       `json:"avg_cost"`
	TotalTokens int           `json:"total_tokens"`
	AvgDuration time.Duration `json:"avg_duration"`
	SuccessRate float64       `json:"success_rate"` // 0.0-1.0
}

// RunCostSummary summarizes cost for a run.
type RunCostSummary struct {
	RunID     string    `json:"run_id"`
	GraphID   string    `json:"graph_id"`
	Cost      float64   `json:"cost"`
	Tokens    int       `json:"tokens"`
	Timestamp time.Time `json:"timestamp"`
}

// RunDurationSummary summarizes duration for a run.
type RunDurationSummary struct {
	RunID     string        `json:"run_id"`
	GraphID   string        `json:"graph_id"`
	Duration  time.Duration `json:"duration"`
	Timestamp time.Time     `json:"timestamp"`
}

// AnalyticsQuery defines query parameters for analytics.
type AnalyticsQuery struct {
	StartTime *time.Time `json:"start_time,omitempty"`
	EndTime   *time.Time `json:"end_time,omitempty"`
	GraphID   string     `json:"graph_id,omitempty"`
	Status    string     `json:"status,omitempty"`
	Limit     int        `json:"limit,omitempty"`
	GroupBy   string     `json:"group_by,omitempty"` // hour, day, week, month
}
