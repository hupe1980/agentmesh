package viz

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"sync"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
)

// CheckpointLoader enables time-travel through execution history.
// It manages loading and navigating through checkpoints for debugging,
// with caching support for performance.
type CheckpointLoader struct {
	checkpointer checkpoint.Checkpointer
	eventStore   *EventStore
	mu           sync.RWMutex
	cache        map[string]*checkpoint.Checkpoint
}

// NewCheckpointLoader creates a new checkpoint loader for time-travel debugging.
func NewCheckpointLoader(checkpointer checkpoint.Checkpointer, eventStore *EventStore) *CheckpointLoader {
	return &CheckpointLoader{
		checkpointer: checkpointer,
		eventStore:   eventStore,
		cache:        make(map[string]*checkpoint.Checkpoint),
	}
}

// CheckpointInfo contains summary information about a checkpoint for timeline display.
type CheckpointInfo struct {
	RunID          string                 `json:"runID"`
	Superstep      int64                  `json:"superstep"`
	Version        uint64                 `json:"version"`
	Timestamp      string                 `json:"timestamp"`
	Committed      bool                   `json:"committed"`
	StateKeys      []string               `json:"stateKeys"`
	CompletedNodes int                    `json:"completedNodes"`
	PausedNodes    []string               `json:"pausedNodes,omitempty"`
	PendingWrites  int                    `json:"pendingWrites"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// GetCheckpointTimeline returns a list of all checkpoints for a run, ordered by superstep.
// This provides the timeline view for time-travel debugging.
func (cl *CheckpointLoader) GetCheckpointTimeline(ctx context.Context, runID string) ([]CheckpointInfo, error) {
	checkpoints, err := cl.checkpointer.List(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to list checkpoints: %w", err)
	}

	// Convert to CheckpointInfo with sorted order (oldest first for timeline)
	infos := make([]CheckpointInfo, 0, len(checkpoints))
	for _, cp := range checkpoints {
		// Extract state keys
		stateKeys := make([]string, 0, len(cp.State))
		for key := range cp.State {
			stateKeys = append(stateKeys, key)
		}
		sort.Strings(stateKeys)

		info := CheckpointInfo{
			RunID:          cp.RunID,
			Superstep:      cp.Superstep,
			Version:        cp.Version,
			Timestamp:      cp.Timestamp.Format("2006-01-02T15:04:05.000Z"),
			Committed:      cp.Committed,
			StateKeys:      stateKeys,
			CompletedNodes: len(cp.CompletedNodes),
			PausedNodes:    cp.PausedNodes,
			PendingWrites:  len(cp.PendingWrites),
			Metadata:       cp.Metadata,
		}
		infos = append(infos, info)
	}

	// Sort by superstep (oldest first)
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Superstep < infos[j].Superstep
	})

	return infos, nil
}

// LoadCheckpoint loads a checkpoint for a specific run at a specific superstep.
// Results are cached for performance.
func (cl *CheckpointLoader) LoadCheckpoint(ctx context.Context, runID string, superstep int64) (*checkpoint.Checkpoint, error) {
	cl.mu.RLock()
	cacheKey := checkpointCacheKey(runID, superstep)
	if cached, exists := cl.cache[cacheKey]; exists {
		cl.mu.RUnlock()
		return cached, nil
	}
	cl.mu.RUnlock()

	// Load from checkpointer using LoadAtSuperstep
	cp, err := cl.checkpointer.LoadAtSuperstep(ctx, runID, superstep)
	if err != nil {
		return nil, fmt.Errorf("failed to load checkpoint at superstep %d: %w", superstep, err)
	}

	if cp == nil {
		return nil, fmt.Errorf("no checkpoint found at superstep %d", superstep)
	}

	// Cache the result
	cl.mu.Lock()
	cl.cache[cacheKey] = cp
	cl.mu.Unlock()

	return cp, nil
}

// NavigateCheckpoint finds the checkpoint relative to the current superstep.
// direction: "prev" for previous checkpoint, "next" for next checkpoint
// Returns the checkpoint info or error if not found.
func (cl *CheckpointLoader) NavigateCheckpoint(ctx context.Context, runID string, currentSuperstep int64, direction string) (*CheckpointInfo, error) {
	timeline, err := cl.GetCheckpointTimeline(ctx, runID)
	if err != nil {
		return nil, err
	}

	if len(timeline) == 0 {
		return nil, fmt.Errorf("no checkpoints found for run %s", runID)
	}

	// Find current position
	currentIdx := -1
	for i := range timeline {
		if timeline[i].Superstep == currentSuperstep {
			currentIdx = i
			break
		}
	}

	if currentIdx == -1 {
		return nil, fmt.Errorf("current superstep %d not found in timeline", currentSuperstep)
	}

	// Navigate
	var targetIdx int
	switch direction {
	case "prev", "backward":
		targetIdx = currentIdx - 1
		if targetIdx < 0 {
			return nil, fmt.Errorf("already at first checkpoint")
		}
	case "next", "forward":
		targetIdx = currentIdx + 1
		if targetIdx >= len(timeline) {
			return nil, fmt.Errorf("already at last checkpoint")
		}
	default:
		return nil, fmt.Errorf("invalid direction: %s (use 'prev' or 'next')", direction)
	}

	return &timeline[targetIdx], nil
}

// GetSuperstepRange returns the min and max supersteps available for a run.
// Useful for timeline scrubbing UI.
func (cl *CheckpointLoader) GetSuperstepRange(ctx context.Context, runID string) (minStep, maxStep int64, err error) {
	timeline, err := cl.GetCheckpointTimeline(ctx, runID)
	if err != nil {
		return 0, 0, err
	}

	if len(timeline) == 0 {
		return 0, 0, fmt.Errorf("no checkpoints found for run %s", runID)
	}

	return timeline[0].Superstep, timeline[len(timeline)-1].Superstep, nil
}

// GetEventsAtStep retrieves events for a specific step
func (cl *CheckpointLoader) GetEventsAtStep(runID string, superstep int64) ([]ExecutionEvent, error) {
	return cl.eventStore.GetEvents(runID, superstep)
}

// ClearCache clears the checkpoint cache
func (cl *CheckpointLoader) ClearCache() {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	cl.cache = make(map[string]*checkpoint.Checkpoint)
}

// checkpointCacheKey generates a cache key
func checkpointCacheKey(runID string, superstep int64) string {
	return runID + ":" + strconv.FormatInt(superstep, 10)
}
