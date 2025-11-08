package graph

import (
	"sort"
	"sync"
)

// TopologyScheduler handles pure DAG dependency resolution through in-degree tracking.
// It maintains a ready queue for O(1) ready vertex access instead of O(n) iteration.
// Thread-safe and maintains no conditional or pause state.
//
// Performance:
//   - Ready():        O(1) - returns pre-maintained ready queue
//   - MarkExecuted(): O(d) where d is out-degree of executed vertex
//   - Previous impl:  O(n) per Ready() call where n is total vertices
type TopologyScheduler struct {
	mu         sync.RWMutex
	incoming   map[string]int  // remaining dependencies per vertex
	baseline   map[string]int  // initial dependency count for reset
	executed   map[string]bool // completed vertices
	readyQueue []string        // maintained list of vertices with incoming=0 and not executed
	inQueue    map[string]bool // fast lookup for queue membership
}

// NewTopologyScheduler creates a scheduler from a compiled graph topology.
// Initializes the ready queue with all vertices that have no dependencies.
func NewTopologyScheduler(incoming map[string]int) *TopologyScheduler {
	baseline := make(map[string]int, len(incoming))
	for name, count := range incoming {
		baseline[name] = count
	}

	ts := &TopologyScheduler{
		incoming:   copyIntMap(incoming),
		baseline:   baseline,
		executed:   make(map[string]bool),
		readyQueue: make([]string, 0),
		inQueue:    make(map[string]bool),
	}

	// Initialize ready queue with vertices that have no dependencies
	for name, count := range ts.incoming {
		if count == 0 {
			ts.readyQueue = append(ts.readyQueue, name)
			ts.inQueue[name] = true
		}
	}
	sort.Strings(ts.readyQueue)

	return ts
}

// Ready returns vertices with all dependencies satisfied.
// O(1) operation - returns maintained ready queue.
func (ts *TopologyScheduler) Ready() []string {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	// Filter out executed vertices from the ready queue
	result := make([]string, 0, len(ts.readyQueue))
	for _, name := range ts.readyQueue {
		if !ts.executed[name] {
			result = append(result, name)
		}
	}
	return result
}

// MarkExecuted records a vertex as completed and decrements dependent vertices.
// Maintains the ready queue: adds newly-ready downstream vertices.
// O(d log n) where d is out-degree and n is ready queue size.
func (ts *TopologyScheduler) MarkExecuted(name string, downstream []string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	ts.executed[name] = true

	// Remove from ready queue if present
	delete(ts.inQueue, name)

	// Process downstream vertices
	newlyReady := make([]string, 0, len(downstream))
	for _, target := range downstream {
		if count, ok := ts.incoming[target]; ok {
			if count > 0 {
				ts.incoming[target] = count - 1
			}
			// If target is now ready and not already executed, add to queue
			if ts.incoming[target] == 0 && !ts.executed[target] && !ts.inQueue[target] {
				newlyReady = append(newlyReady, target)
				ts.inQueue[target] = true
			}
		}
	}

	// Add newly ready vertices to queue and maintain sorted order
	if len(newlyReady) > 0 {
		ts.readyQueue = append(ts.readyQueue, newlyReady...)
		sort.Strings(ts.readyQueue)
	}
}

// Reset clears execution state for restart and rebuilds ready queue.
func (ts *TopologyScheduler) Reset() {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	ts.incoming = copyIntMap(ts.baseline)
	for k := range ts.executed {
		delete(ts.executed, k)
	}

	// Rebuild ready queue from baseline
	ts.readyQueue = ts.readyQueue[:0] // reuse capacity
	for k := range ts.inQueue {
		delete(ts.inQueue, k)
	}

	for name, count := range ts.incoming {
		if count == 0 {
			ts.readyQueue = append(ts.readyQueue, name)
			ts.inQueue[name] = true
		}
	}
	sort.Strings(ts.readyQueue)
}

// SetExecuted marks specific vertices as completed (for bootstrap/resume).
// Maintains ready queue by adding newly-ready downstream vertices.
func (ts *TopologyScheduler) SetExecuted(names []string, downstream map[string][]string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	newlyReady := make([]string, 0)

	for _, name := range names {
		if !ts.executed[name] {
			ts.executed[name] = true
			delete(ts.inQueue, name)

			for _, target := range downstream[name] {
				if count, ok := ts.incoming[target]; ok && count > 0 {
					ts.incoming[target] = count - 1

					// Add to ready queue if dependencies satisfied
					if ts.incoming[target] == 0 && !ts.executed[target] && !ts.inQueue[target] {
						newlyReady = append(newlyReady, target)
						ts.inQueue[target] = true
					}
				}
			}
		}
	}

	// Add newly ready vertices and maintain sorted order
	if len(newlyReady) > 0 {
		ts.readyQueue = append(ts.readyQueue, newlyReady...)
		sort.Strings(ts.readyQueue)
	}
}

func copyIntMap(m map[string]int) map[string]int {
	result := make(map[string]int, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}
