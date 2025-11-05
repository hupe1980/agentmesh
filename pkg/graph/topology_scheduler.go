package graph

import (
	"sort"
	"sync"
)

// TopologyScheduler handles pure DAG dependency resolution through in-degree tracking.
// It is thread-safe and maintains no conditional or pause state.
type TopologyScheduler struct {
	mu       sync.RWMutex
	incoming map[string]int  // remaining dependencies
	baseline map[string]int  // initial dependency count
	executed map[string]bool // completed vertices
}

// NewTopologyScheduler creates a scheduler from a compiled graph topology.
func NewTopologyScheduler(incoming map[string]int) *TopologyScheduler {
	baseline := make(map[string]int, len(incoming))
	for name, count := range incoming {
		baseline[name] = count
	}
	return &TopologyScheduler{
		incoming: copyIntMap(incoming),
		baseline: baseline,
		executed: make(map[string]bool),
	}
}

// Ready returns vertices with all dependencies satisfied.
func (ts *TopologyScheduler) Ready() []string {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	ready := make([]string, 0)
	for name, remaining := range ts.incoming {
		if remaining == 0 && !ts.executed[name] {
			ready = append(ready, name)
		}
	}
	sort.Strings(ready)
	return ready
}

// MarkExecuted records a vertex as completed and decrements dependent vertices.
func (ts *TopologyScheduler) MarkExecuted(name string, downstream []string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	ts.executed[name] = true
	for _, target := range downstream {
		if count, ok := ts.incoming[target]; ok {
			if count > 0 {
				ts.incoming[target] = count - 1
			}
			// If target is now ready (or was already at 0), clear its executed flag so it can run again
			if ts.incoming[target] == 0 {
				delete(ts.executed, target)
			}
		}
	}
}

// Reset clears execution state for restart.
func (ts *TopologyScheduler) Reset() {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	ts.incoming = copyIntMap(ts.baseline)
	for k := range ts.executed {
		delete(ts.executed, k)
	}
}

// SetExecuted marks specific vertices as completed (for bootstrap/resume).
func (ts *TopologyScheduler) SetExecuted(names []string, downstream map[string][]string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	for _, name := range names {
		if !ts.executed[name] {
			ts.executed[name] = true
			for _, target := range downstream[name] {
				if count, ok := ts.incoming[target]; ok && count > 0 {
					ts.incoming[target] = count - 1
				}
			}
		}
	}
}

func copyIntMap(m map[string]int) map[string]int {
	result := make(map[string]int, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}
