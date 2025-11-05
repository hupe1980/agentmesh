package graph

// This file provides built-in aggregator implementations for common use cases.
// Aggregators enable distributed computation patterns like global counters,
// max/min tracking, and convergence detection.

// SumAggregator sums numeric values across all nodes.
// Useful for counting events, computing totals, etc.
//
// Example:
//
//	compiled.Invoke(ctx, messages, WithAggregators(map[string]Aggregator{
//	    "total_processed": &SumAggregator{},
//	}))
//
//	// In node:
//	s.Aggregate("total_processed", 1)  // Increment counter
type SumAggregator struct{}

func (a *SumAggregator) Zero() any {
	return 0
}

func (a *SumAggregator) Aggregate(current, value any) any {
	// Convert to float64 for flexibility
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
// Useful for finding highest priority, largest cost, etc.
//
// Example:
//
//	compiled.Invoke(ctx, messages, WithAggregators(map[string]Aggregator{
//	    "max_priority": &MaxAggregator{},
//	}))
//
//	// In node:
//	s.Aggregate("max_priority", taskPriority)
type MaxAggregator struct{}

func (a *MaxAggregator) Zero() any {
	return float64(-1e308) // Min float64
}

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
//	compiled.Invoke(ctx, messages, WithAggregators(map[string]Aggregator{
//	    "min_cost": &MinAggregator{},
//	}))
type MinAggregator struct{}

func (a *MinAggregator) Zero() any {
	return float64(1e308) // Max float64
}

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
//	compiled.Invoke(ctx, messages, WithAggregators(map[string]Aggregator{
//	    "active_nodes": &CountAggregator{},
//	}))
//
//	// In node:
//	s.Aggregate("active_nodes", true)  // Any non-nil value increments
type CountAggregator struct{}

func (a *CountAggregator) Zero() any {
	return 0
}

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
//	compiled.Invoke(ctx, messages, WithAggregators(map[string]Aggregator{
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

func (a *AllTrueAggregator) Zero() any {
	return true
}

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
//	compiled.Invoke(ctx, messages, WithAggregators(map[string]Aggregator{
//	    "has_errors": &AnyTrueAggregator{},
//	}))
//
//	// In node:
//	if err := process(); err != nil {
//	    s.Aggregate("has_errors", true)
//	}
type AnyTrueAggregator struct{}

func (a *AnyTrueAggregator) Zero() any {
	return false
}

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
//	compiled.Invoke(ctx, messages, WithAggregators(map[string]Aggregator{
//	    "node_trace": &StringConcatAggregator{Separator: " -> "},
//	}))
type StringConcatAggregator struct {
	Separator string
}

func (a *StringConcatAggregator) Zero() any {
	return ""
}

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
