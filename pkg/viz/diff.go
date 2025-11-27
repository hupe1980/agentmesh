package viz

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
)

// DiffType represents the type of change in a state diff
type DiffType string

// Diff type constants
const (
	// DiffTypeAdded indicates a value was added
	DiffTypeAdded DiffType = "added"
	// DiffTypeRemoved indicates a value was removed
	DiffTypeRemoved DiffType = "removed"
	// DiffTypeModified indicates a value was changed
	DiffTypeModified DiffType = "modified"
	// DiffTypeNone indicates no change
	DiffTypeNone DiffType = "none"
)

// StateDiff represents a difference between two state values
type StateDiff struct {
	Type     DiffType    `json:"type"`
	Key      string      `json:"key"`
	OldValue interface{} `json:"oldValue,omitempty"`
	NewValue interface{} `json:"newValue,omitempty"`
	Changed  bool        `json:"changed"`
}

// CheckpointDiff represents the differences between two checkpoints
type CheckpointDiff struct {
	FromSuperstep  int64                  `json:"fromSuperstep"`
	ToSuperstep    int64                  `json:"toSuperstep"`
	StateDiffs     []StateDiff            `json:"stateDiffs"`
	NodesCompleted []string               `json:"nodesCompleted,omitempty"` // Nodes completed between checkpoints
	NodesPaused    []string               `json:"nodesPaused,omitempty"`    // Nodes paused between checkpoints
	WritesApplied  int                    `json:"writesApplied"`            // PendingWrites that were committed
	Summary        map[string]interface{} `json:"summary"`
}

// StateDiffer computes differences between checkpoint states for time-travel debugging.
type StateDiffer struct{}

// NewStateDiffer creates a new state differ.
func NewStateDiffer() *StateDiffer {
	return &StateDiffer{}
}

// ComputeDiff calculates the differences between two checkpoints.
// This is the primary method for visualizing state changes over time.
func (sd *StateDiffer) ComputeDiff(from, to *checkpoint.Checkpoint) (*CheckpointDiff, error) {
	if from == nil || to == nil {
		return nil, fmt.Errorf("cannot compute diff: nil checkpoint provided")
	}

	diff := &CheckpointDiff{
		FromSuperstep: from.Superstep,
		ToSuperstep:   to.Superstep,
		StateDiffs:    []StateDiff{},
		Summary:       make(map[string]interface{}),
	}

	// Compute state diffs
	stateDiffs := sd.computeStateDiffs(from.State, to.State)
	diff.StateDiffs = stateDiffs

	// Track node completion changes
	diff.NodesCompleted = sd.findNewItems(from.CompletedNodes, to.CompletedNodes)
	diff.NodesPaused = sd.findNewItems(from.PausedNodes, to.PausedNodes)

	// Track pending writes that were committed
	if from.Committed != to.Committed && to.Committed {
		diff.WritesApplied = len(from.PendingWrites)
	}

	// Generate summary
	diff.Summary = map[string]interface{}{
		"stateChanges":      countChangedDiffs(stateDiffs),
		"stateAdditions":    countDiffsByType(stateDiffs, DiffTypeAdded),
		"stateRemovals":     countDiffsByType(stateDiffs, DiffTypeRemoved),
		"nodesCompleted":    len(diff.NodesCompleted),
		"nodesPaused":       len(diff.NodesPaused),
		"pendingWrites":     len(to.PendingWrites),
		"versionIncrement":  safeVersionDiff(from.Version, to.Version),
		"superstepDistance": to.Superstep - from.Superstep,
	}

	return diff, nil
}

// computeStateDiffs compares two state maps and returns the differences
func (sd *StateDiffer) computeStateDiffs(oldState, newState map[string]any) []StateDiff {
	diffs := []StateDiff{}

	// Find all unique keys
	allKeys := make(map[string]bool)
	for key := range oldState {
		allKeys[key] = true
	}
	for key := range newState {
		allKeys[key] = true
	}

	// Sort keys for consistent output
	keys := make([]string, 0, len(allKeys))
	for key := range allKeys {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	// Compare each key
	for _, key := range keys {
		oldVal, oldExists := oldState[key]
		newVal, newExists := newState[key]

		var diff StateDiff
		diff.Key = key

		switch {
		case !oldExists && newExists:
			// Key added
			diff.Type = DiffTypeAdded
			diff.NewValue = newVal
			diff.Changed = true

		case oldExists && !newExists:
			// Key removed
			diff.Type = DiffTypeRemoved
			diff.OldValue = oldVal
			diff.Changed = true

		case oldExists && newExists:
			// Key exists in both - check if value changed
			if !sd.valuesEqual(oldVal, newVal) {
				diff.Type = DiffTypeModified
				diff.OldValue = oldVal
				diff.NewValue = newVal
				diff.Changed = true
			} else {
				diff.Type = DiffTypeNone
				diff.OldValue = oldVal
				diff.NewValue = newVal
				diff.Changed = false
			}
		}

		diffs = append(diffs, diff)
	}

	return diffs
}

// valuesEqual compares two values for equality.
// Uses deep equality for complex types.
func (sd *StateDiffer) valuesEqual(a, b interface{}) bool {
	// Try direct comparison first
	if reflect.DeepEqual(a, b) {
		return true
	}

	// For complex types, compare JSON representations
	// This handles cases where pointer addresses differ but content is the same
	aJSON, aErr := json.Marshal(a)
	bJSON, bErr := json.Marshal(b)

	if aErr != nil || bErr != nil {
		// If marshaling fails, fall back to reflect.DeepEqual result
		return false
	}

	return bytes.Equal(aJSON, bJSON)
}

// safeVersionDiff safely computes version difference with overflow protection
func safeVersionDiff(from, to uint64) int64 {
	if to >= from {
		diff := to - from
		if diff > math.MaxInt64 {
			return math.MaxInt64
		}
		return int64(diff)
	}
	// Negative diff (version decreased, shouldn't happen but handle it)
	diff := from - to
	if diff > math.MaxInt64 {
		return -math.MaxInt64
	}
	return -int64(diff)
}

// findNewItems returns items in newList that aren't in oldList
func (sd *StateDiffer) findNewItems(oldList, newList []string) []string {
	oldSet := make(map[string]bool)
	for _, item := range oldList {
		oldSet[item] = true
	}

	newItems := []string{}
	for _, item := range newList {
		if !oldSet[item] {
			newItems = append(newItems, item)
		}
	}

	return newItems
}

// Helper functions for summary statistics

func countChangedDiffs(diffs []StateDiff) int {
	count := 0
	for _, diff := range diffs {
		if diff.Changed {
			count++
		}
	}
	return count
}

func countDiffsByType(diffs []StateDiff, diffType DiffType) int {
	count := 0
	for _, diff := range diffs {
		if diff.Type == diffType {
			count++
		}
	}
	return count
}

// ComputeStateSnapshot extracts a human-readable snapshot of checkpoint state.
// Useful for displaying state at a specific point in time.
func (sd *StateDiffer) ComputeStateSnapshot(cp *checkpoint.Checkpoint) map[string]interface{} {
	snapshot := map[string]interface{}{
		"superstep":      cp.Superstep,
		"version":        cp.Version,
		"timestamp":      cp.Timestamp.Format("2006-01-02T15:04:05.000Z"),
		"committed":      cp.Committed,
		"stateKeys":      sd.extractStateKeys(cp.State),
		"completedNodes": cp.CompletedNodes,
		"pausedNodes":    cp.PausedNodes,
		"pendingWrites":  len(cp.PendingWrites),
	}

	// Add state values (limited to prevent huge payloads)
	statePreview := make(map[string]interface{})
	for key, value := range cp.State {
		statePreview[key] = sd.truncateValue(value, 100)
	}
	snapshot["state"] = statePreview

	return snapshot
}

// extractStateKeys returns sorted list of state keys
func (sd *StateDiffer) extractStateKeys(state map[string]any) []string {
	keys := make([]string, 0, len(state))
	for key := range state {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// truncateValue truncates large values for preview purposes
func (sd *StateDiffer) truncateValue(value interface{}, maxLen int) interface{} {
	// Convert to JSON to check size
	jsonBytes, err := json.Marshal(value)
	if err != nil {
		return "<error serializing>"
	}

	if len(jsonBytes) <= maxLen {
		return value
	}

	// Truncate string representation
	truncated := string(jsonBytes)
	if len(truncated) > maxLen {
		truncated = truncated[:maxLen] + "..."
	}

	return truncated
}
