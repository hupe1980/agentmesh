package pregel

import (
	"context"
	"sort"
	"sync"
)

// PriorityScheduler executes vertices based on priority values.
// Higher priority vertices are executed first within the worker pool.
//
// ALGORITHM:
//   - Sorts vertices by priority (descending)
//   - Stable sort preserves lexicographic order for equal priorities
//   - O(n log n) where n is frontier size
//
// USE CASES:
//   - Critical path optimization (prioritize blocking nodes)
//   - Cost-based execution (expensive operations first/last)
//   - User-defined importance (VIP requests first)
type PriorityScheduler struct {
	mu              sync.RWMutex
	priorities      map[string]int
	defaultPriority int
}

// NewPriorityScheduler creates a scheduler that orders vertices by priority.
// Higher priority values execute first. Vertices not in the priority map
// use defaultPriority.
//
// Example:
//
//	priorities := map[string]int{
//	    "critical_node": 100,
//	    "normal_node":   50,
//	    "background":    10,
//	}
//	scheduler := NewPriorityScheduler(priorities, 50) // default=50
func NewPriorityScheduler(priorities map[string]int, defaultPriority int) *PriorityScheduler {
	// Create defensive copy to prevent external mutation
	prioritiesCopy := make(map[string]int, len(priorities))
	for k, v := range priorities {
		prioritiesCopy[k] = v
	}

	return &PriorityScheduler{
		priorities:      prioritiesCopy,
		defaultPriority: defaultPriority,
	}
}

// NextBatch returns vertices sorted by priority (descending).
// Ties are broken lexicographically for determinism.
func (s *PriorityScheduler) NextBatch(ctx context.Context, info SchedulerInfo) ([]string, error) {
	if len(info.Frontier) == 0 {
		return []string{}, nil
	}

	// Extract frontier vertices
	batch := make([]string, 0, len(info.Frontier))
	for vertex := range info.Frontier {
		batch = append(batch, vertex)
	}

	// Sort by priority (descending), then lexicographically
	s.mu.RLock()
	sort.SliceStable(batch, func(i, j int) bool {
		priI := s.getPriority(batch[i])
		priJ := s.getPriority(batch[j])
		if priI != priJ {
			return priI > priJ // Higher priority first
		}
		return batch[i] < batch[j] // Lexicographic for ties
	})
	s.mu.RUnlock()

	return batch, nil
}

// getPriority returns the priority for a vertex (caller must hold read lock).
func (s *PriorityScheduler) getPriority(vertex string) int {
	if pri, ok := s.priorities[vertex]; ok {
		return pri
	}
	return s.defaultPriority
}

// RecordCompletion is a no-op for priority scheduler (priorities are static).
func (s *PriorityScheduler) RecordCompletion(ctx context.Context, vertex string, info CompletionInfo) {
	// No-op: Priorities are configured upfront, not learned
}

// SetPriority updates the priority for a vertex.
// Safe to call concurrently. Changes affect subsequent supersteps.
func (s *PriorityScheduler) SetPriority(vertex string, priority int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.priorities[vertex] = priority
}

// GetPriority returns the current priority for a vertex.
// Safe to call concurrently.
func (s *PriorityScheduler) GetPriority(vertex string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getPriority(vertex)
}

// Ensure PriorityScheduler implements Scheduler interface
var _ Scheduler = (*PriorityScheduler)(nil)

// ResourceAwareScheduler orders vertices based on available resources.
// Executes vertices with lower resource requirements first to maximize
// parallelism and prevent resource exhaustion.
//
// ALGORITHM:
//   - Sorts by estimated resource usage (ascending)
//   - Can consider memory, CPU, I/O, or custom metrics
//   - Falls back to lexicographic order for ties
//   - O(n log n) where n is frontier size
//
// USE CASES:
//   - Memory-constrained environments (small tasks first)
//   - CPU-bound workloads (distribute load evenly)
//   - Mixed workload optimization (I/O vs CPU tasks)
type ResourceAwareScheduler struct {
	mu            sync.RWMutex
	resourceCosts map[string]int // Estimated resource units per vertex
	defaultCost   int
	preferLowCost bool // true = low-cost first, false = high-cost first
}

// NewResourceAwareScheduler creates a scheduler that orders by resource cost.
// If preferLowCost=true, executes low-cost vertices first (maximize parallelism).
// If preferLowCost=false, executes high-cost vertices first (reduce tail latency).
//
// Example:
//
//	costs := map[string]int{
//	    "llm_call":      100, // Expensive
//	    "validation":    10,  // Cheap
//	    "data_fetch":    50,  // Medium
//	}
//	scheduler := NewResourceAwareScheduler(costs, 25, true) // low-cost first
func NewResourceAwareScheduler(resourceCosts map[string]int, defaultCost int, preferLowCost bool) *ResourceAwareScheduler {
	// Defensive copy
	costsCopy := make(map[string]int, len(resourceCosts))
	for k, v := range resourceCosts {
		costsCopy[k] = v
	}

	return &ResourceAwareScheduler{
		resourceCosts: costsCopy,
		defaultCost:   defaultCost,
		preferLowCost: preferLowCost,
	}
}

// NextBatch returns vertices sorted by resource cost.
// Order depends on preferLowCost: ascending (true) or descending (false).
func (s *ResourceAwareScheduler) NextBatch(ctx context.Context, info SchedulerInfo) ([]string, error) {
	if len(info.Frontier) == 0 {
		return []string{}, nil
	}

	// Extract frontier vertices
	batch := make([]string, 0, len(info.Frontier))
	for vertex := range info.Frontier {
		batch = append(batch, vertex)
	}

	// Sort by resource cost
	s.mu.RLock()
	sort.SliceStable(batch, func(i, j int) bool {
		costI := s.getResourceCost(batch[i])
		costJ := s.getResourceCost(batch[j])
		if costI != costJ {
			if s.preferLowCost {
				return costI < costJ // Low cost first
			}
			return costI > costJ // High cost first
		}
		return batch[i] < batch[j] // Lexicographic for ties
	})
	s.mu.RUnlock()

	return batch, nil
}

// getResourceCost returns the resource cost for a vertex (caller must hold read lock).
func (s *ResourceAwareScheduler) getResourceCost(vertex string) int {
	if cost, ok := s.resourceCosts[vertex]; ok {
		return cost
	}
	return s.defaultCost
}

// RecordCompletion is a no-op for resource-aware scheduler (costs are static).
func (s *ResourceAwareScheduler) RecordCompletion(ctx context.Context, vertex string, info CompletionInfo) {
	// No-op: Resource costs are configured upfront, not learned
	// Future: Could implement adaptive cost estimation based on actual resource usage
}

// SetResourceCost updates the resource cost for a vertex.
// Safe to call concurrently. Changes affect subsequent supersteps.
func (s *ResourceAwareScheduler) SetResourceCost(vertex string, cost int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resourceCosts[vertex] = cost
}

// GetResourceCost returns the current resource cost for a vertex.
// Safe to call concurrently.
func (s *ResourceAwareScheduler) GetResourceCost(vertex string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getResourceCost(vertex)
}

// Ensure ResourceAwareScheduler implements Scheduler interface
var _ Scheduler = (*ResourceAwareScheduler)(nil)
