package graph

// This file provides built-in aggregator implementations for common use cases.
// Aggregators enable distributed computation patterns like global counters,
// max/min tracking, convergence detection, and statistical analysis.
//
// All aggregators follow the BSP (Bulk Synchronous Parallel) model:
//   - Values contributed in superstep N are aggregated
//   - The final aggregated value becomes visible in superstep N+1
//   - Aggregates are accessible via state.AggregatesSnapshot()
//
// Thread Safety:
// All aggregator implementations are stateless (zero-cost structs) and
// thread-safe. The runtime handles synchronization of aggregate values.
//
// Custom Aggregators:
// Implement the pregel.Aggregator interface to create custom aggregation logic:
//
//	type pregel.Aggregator interface {
//	    Zero() any                        // Initial/identity value
//	    Aggregate(current, value any) any // Combine current with new value
//	}

// SumAggregator sums numeric values across all nodes.
// Useful for counting events, computing totals, tracking metrics.
//
// Supports: int, int64, float32, float64
// Returns: float64
//
// Example:
//
//	compiled.Invoke(ctx, messages, WithAggregators(map[string]pregel.Aggregator{
//	    "total_processed": &SumAggregator{},
//	}))
//
//	// In node:
//	s.Aggregate("total_processed", 1)  // Increment counter
//
//	// After superstep:
//	total := s.AggregatesSnapshot()["total_processed"].(float64)
type SumAggregator struct{}

// Zero returns the identity value for summation.
func (a *SumAggregator) Zero() any {
	return 0
}

// Aggregate adds values together.
func (a *SumAggregator) Aggregate(current, value any) any {
	// Convert to float64 for numerical flexibility and to avoid overflow
	var curVal, newVal float64
	switch v := current.(type) {
	case int:
		curVal = float64(v)
	case int64:
		curVal = float64(v)
	case float64:
		curVal = v
	case float32:
		curVal = float64(v)
	default:
		curVal = 0
	}

	switch v := value.(type) {
	case int:
		newVal = float64(v)
	case int64:
		newVal = float64(v)
	case float64:
		newVal = v
	case float32:
		newVal = float64(v)
	default:
		newVal = 0
	}

	return curVal + newVal
}

// MaxAggregator tracks the maximum value across all nodes.
// Useful for finding highest priority, largest cost, peak values.
//
// Supports: int, int64, float32, float64
// Returns: float64
// Zero value: -1e308 (smallest float64)
//
// Example:
//
//	compiled.Invoke(ctx, messages, WithAggregators(map[string]pregel.Aggregator{
//	    "max_priority": &MaxAggregator{},
//	}))
//
//	// In node:
//	s.Aggregate("max_priority", taskPriority)
//
//	// After superstep:
//	maxPriority := s.AggregatesSnapshot()["max_priority"].(float64)
type MaxAggregator struct{}

// Zero returns the minimum possible float64 value.
func (a *MaxAggregator) Zero() any {
	return float64(-1e308) // Smallest possible float64
}

// Aggregate returns the maximum of current and value.
func (a *MaxAggregator) Aggregate(current, value any) any {
	var curVal, newVal float64

	switch v := current.(type) {
	case int:
		curVal = float64(v)
	case int64:
		curVal = float64(v)
	case float64:
		curVal = v
	case float32:
		curVal = float64(v)
	default:
		curVal = float64(-1e308)
	}

	switch v := value.(type) {
	case int:
		newVal = float64(v)
	case int64:
		newVal = float64(v)
	case float64:
		newVal = v
	case float32:
		newVal = float64(v)
	default:
		return curVal
	}

	if newVal > curVal {
		return newVal
	}
	return curVal
}

// MinAggregator tracks the minimum value across all nodes.
// Useful for finding lowest cost, shortest path, etc.
//
// Example:
//
//	compiled.Invoke(ctx, messages, WithAggregators(map[string]pregel.Aggregator{
//	    "min_cost": &MinAggregator{},
//	}))
type MinAggregator struct{}

// Zero returns the maximum possible float64 value.
func (a *MinAggregator) Zero() any {
	return float64(1e308) // Max float64
}

// Aggregate returns the minimum of current and value.
func (a *MinAggregator) Aggregate(current, value any) any {
	var curVal, newVal float64

	switch v := current.(type) {
	case int:
		curVal = float64(v)
	case int64:
		curVal = float64(v)
	case float64:
		curVal = v
	case float32:
		curVal = float64(v)
	default:
		curVal = float64(1e308)
	}

	switch v := value.(type) {
	case int:
		newVal = float64(v)
	case int64:
		newVal = float64(v)
	case float64:
		newVal = v
	case float32:
		newVal = float64(v)
	default:
		return curVal
	}

	if newVal < curVal {
		return newVal
	}
	return curVal
}

// CountAggregator counts non-nil aggregation calls.
// Useful for counting active nodes, completed tasks, etc.
//
// Example:
//
//	compiled.Invoke(ctx, messages, WithAggregators(map[string]pregel.Aggregator{
//	    "active_nodes": &CountAggregator{},
//	}))
//
//	// In node:
//	s.Aggregate("active_nodes", true)  // Any non-nil value increments
type CountAggregator struct{}

// Zero returns the initial count of zero.
func (a *CountAggregator) Zero() any {
	return 0
}

// Aggregate increments the count.
func (a *CountAggregator) Aggregate(current, value any) any {
	curCount, ok := current.(int)
	if !ok {
		curCount = 0
	}
	if value != nil {
		return curCount + 1
	}
	return curCount
}

// AllTrueAggregator checks if all nodes report true.
// Useful for convergence detection, readiness checks, etc.
//
// Example:
//
//	compiled.Invoke(ctx, messages, WithAggregators(map[string]pregel.Aggregator{
//	    "all_converged": &AllTrueAggregator{},
//	}))
//
//	// In node:
//	converged := checkConvergence()
//	s.Aggregate("all_converged", converged)
//
//	// Check after superstep:
//	if s.AggregatesSnapshot()["all_converged"].(bool) {
//	    // All nodes converged, can terminate early
//	}
type AllTrueAggregator struct{}

// Zero returns true as the identity value.
func (a *AllTrueAggregator) Zero() any {
	return true
}

// Aggregate returns true only if both current and value are true.
func (a *AllTrueAggregator) Aggregate(current, value any) any {
	curVal, ok := current.(bool)
	if !ok {
		curVal = true
	}

	newVal, ok := value.(bool)
	if !ok {
		return false
	}

	return curVal && newVal
}

// AnyTrueAggregator checks if any node reports true.
// Useful for error detection, condition monitoring, etc.
//
// Example:
//
//	compiled.Invoke(ctx, messages, WithAggregators(map[string]pregel.Aggregator{
//	    "has_errors": &AnyTrueAggregator{},
//	}))
//
//	// In node:
//	if err := process(); err != nil {
//	    s.Aggregate("has_errors", true)
//	}
type AnyTrueAggregator struct{}

// Zero returns false as the identity value.
func (a *AnyTrueAggregator) Zero() any {
	return false
}

// Aggregate returns true if either current or value is true.
func (a *AnyTrueAggregator) Aggregate(current, value any) any {
	curVal, ok := current.(bool)
	if !ok {
		curVal = false
	}

	newVal, ok := value.(bool)
	if !ok {
		return curVal
	}

	return curVal || newVal
}

// StringConcatAggregator concatenates strings with a separator.
// Useful for collecting logs, building composite keys, etc.
//
// Example:
//
//	compiled.Invoke(ctx, messages, WithAggregators(map[string]pregel.Aggregator{
//	    "node_trace": &StringConcatAggregator{Separator: " -> "},
//	}))
type StringConcatAggregator struct {
	Separator string
}

// Zero returns an empty string.
func (a *StringConcatAggregator) Zero() any {
	return ""
}

// Aggregate concatenates strings with the configured separator.
func (a *StringConcatAggregator) Aggregate(current, value any) any {
	curStr, _ := current.(string)
	newStr, ok := value.(string)
	if !ok {
		return curStr
	}

	if curStr == "" {
		return newStr
	}
	if newStr == "" {
		return curStr
	}

	sep := a.Separator
	if sep == "" {
		sep = ""
	}

	return curStr + sep + newStr
}

// AvgAggregator computes the average (mean) of numeric values across all nodes.
// Uses Welford's online algorithm for numerical stability.
//
// Example:
//
//	compiled.Invoke(ctx, messages, WithAggregators(map[string]pregel.Aggregator{
//	    "avg_latency": &AvgAggregator{},
//	}))
//
//	// In node:
//	s.Aggregate("avg_latency", responseTime)
//
//	// Check after superstep:
//	avgLatency := s.AggregatesSnapshot()["avg_latency"].(float64)
type AvgAggregator struct{}

// avgState tracks running mean and count
type avgState struct {
	Mean  float64
	Count int64
}

// Zero returns the initial average state.
func (a *AvgAggregator) Zero() any {
	return avgState{Mean: 0, Count: 0}
}

// Aggregate computes running average using Welford's online algorithm.
func (a *AvgAggregator) Aggregate(current, value any) any {
	state, ok := current.(avgState)
	if !ok {
		state = avgState{Mean: 0, Count: 0}
	}

	// Convert value to float64
	var val float64
	switch v := value.(type) {
	case int:
		val = float64(v)
	case int32:
		val = float64(v)
	case int64:
		val = float64(v)
	case float64:
		val = v
	case float32:
		val = float64(v)
	default:
		return state // Ignore invalid values
	}

	// Welford's online algorithm for numerical stability
	state.Count++
	delta := val - state.Mean
	state.Mean += delta / float64(state.Count)

	return state
}

// VarianceAggregator computes the variance of numeric values across all nodes.
// Uses Welford's online algorithm for numerical stability.
//
// Example:
//
//	compiled.Invoke(ctx, messages, WithAggregators(map[string]pregel.Aggregator{
//	    "variance_latency": &VarianceAggregator{},
//	}))
//
//	// In node:
//	s.Aggregate("variance_latency", responseTime)
//
//	// Check after superstep:
//	variance := s.AggregatesSnapshot()["variance_latency"].(float64)
type VarianceAggregator struct{}

// varianceState tracks running variance calculation
type varianceState struct {
	Mean  float64
	M2    float64 // Sum of squared differences from mean
	Count int64
}

// Zero returns the initial variance state.
func (a *VarianceAggregator) Zero() any {
	return varianceState{Mean: 0, M2: 0, Count: 0}
}

// Aggregate computes running variance using Welford's online algorithm.
func (a *VarianceAggregator) Aggregate(current, value any) any {
	state, ok := current.(varianceState)
	if !ok {
		state = varianceState{Mean: 0, M2: 0, Count: 0}
	}

	// Convert value to float64
	var val float64
	switch v := value.(type) {
	case int:
		val = float64(v)
	case int32:
		val = float64(v)
	case int64:
		val = float64(v)
	case float64:
		val = v
	case float32:
		val = float64(v)
	default:
		return state // Ignore invalid values
	}

	// Welford's online algorithm
	state.Count++
	delta := val - state.Mean
	state.Mean += delta / float64(state.Count)
	delta2 := val - state.Mean
	state.M2 += delta * delta2

	return state
}
