package viz

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// EventType represents the type of execution event
type EventType string

// Event type constants
const (
	// EventNodeStart indicates a node has started execution
	EventNodeStart EventType = "node_start"
	// EventNodeComplete indicates a node completed successfully
	EventNodeComplete EventType = "node_complete"
	// EventNodeError indicates a node execution failed
	EventNodeError EventType = "node_error"
	// EventStepStart indicates a step has started
	EventStepStart EventType = "step_start"
	// EventStepEnd indicates a step has ended
	EventStepEnd EventType = "step_end"
	// EventStateUpdate indicates state was updated
	EventStateUpdate EventType = "state_update"
	// EventCheckpoint indicates a checkpoint was saved
	EventCheckpoint EventType = "checkpoint"
	// EventInterrupt indicates execution was interrupted
	EventInterrupt EventType = "interrupt"
)

// EventIndex provides fast lookup for events
type EventIndex struct {
	// Index by event type
	byType map[EventType][]string

	// Index by node name
	byNode map[string][]string

	// Sorted list of event IDs by timestamp
	byTime []string
}

// RunEvents contains all events for a single run
type RunEvents struct {
	RunID     string           `json:"id"`
	Events    []ExecutionEvent `json:"events"`
	StartTime time.Time        `json:"start_time"`
	EndTime   *time.Time       `json:"end_time,omitempty"`
	Status    RunStatus        `json:"status"`
	Index     *EventIndex      `json:"index,omitempty"`
	GraphID   string           `json:"graph_id,omitempty"`
}

// EventStore stores execution events in memory with a circular buffer
type EventStore struct {
	mu      sync.RWMutex
	runs    map[string]*RunEvents
	maxSize int
}

// NewEventStore creates a new event store
func NewEventStore(maxSize int) *EventStore {
	return &EventStore{
		runs:    make(map[string]*RunEvents),
		maxSize: maxSize,
	}
}

// newEventIndex creates a new event index
func newEventIndex() *EventIndex {
	return &EventIndex{
		byType: make(map[EventType][]string),
		byNode: make(map[string][]string),
		byTime: make([]string, 0),
	}
}

// addToIndex adds an event to the index
func (idx *EventIndex) addToIndex(event ExecutionEvent) {
	// Add to type index
	idx.byType[event.Type] = append(idx.byType[event.Type], event.ID)

	// Add to node index if event has a node
	if event.Node != "" {
		idx.byNode[event.Node] = append(idx.byNode[event.Node], event.ID)
	}

	// Add to time index (maintain sorted order)
	idx.byTime = append(idx.byTime, event.ID)
}

// InitRun initializes a run with optional graph ID
// Note: StartTime will be set to the first event's timestamp when events are appended
func (es *EventStore) InitRun(runID, graphID string) {
	es.mu.Lock()
	defer es.mu.Unlock()

	if _, exists := es.runs[runID]; !exists {
		es.runs[runID] = &RunEvents{
			RunID:   runID,
			GraphID: graphID,
			Events:  make([]ExecutionEvent, 0, 100),
			Status:  StatusRunning,
			Index:   newEventIndex(),
		}
	}
}

// Append adds an event to the store
func (es *EventStore) Append(event ExecutionEvent) error {
	es.mu.Lock()
	defer es.mu.Unlock()

	run, exists := es.runs[event.RunID]
	if !exists {
		run = &RunEvents{
			RunID:     event.RunID,
			Events:    make([]ExecutionEvent, 0, 100),
			StartTime: event.Timestamp,
			Status:    StatusRunning,
			Index:     newEventIndex(),
		}
		es.runs[event.RunID] = run
	} else if run.StartTime.IsZero() {
		// Set start time from first event if not already set
		run.StartTime = event.Timestamp
	}

	// Implement circular buffer
	if es.maxSize > 0 && len(run.Events) >= es.maxSize {
		// Remove oldest event
		run.Events = run.Events[1:]
	}

	// Only append if maxSize allows it
	if es.maxSize > 0 {
		run.Events = append(run.Events, event)
		run.Index.addToIndex(event)
	}

	// Update run status based on event type
	switch event.Type {
	case EventNodeComplete:
		if event.Payload.Error != "" {
			run.Status = StatusFailed
			now := time.Now()
			run.EndTime = &now
		}
	case EventNodeError, EventGraphError:
		run.Status = StatusFailed
		now := time.Now()
		run.EndTime = &now
	case EventGraphComplete:
		run.Status = StatusCompleted
		now := time.Now()
		run.EndTime = &now
	}

	return nil
}

// GetEvents retrieves events for a specific run starting from a superstep
func (es *EventStore) GetEvents(runID string, fromStep int64) ([]ExecutionEvent, error) {
	es.mu.RLock()
	defer es.mu.RUnlock()

	run, exists := es.runs[runID]
	if !exists {
		return nil, fmt.Errorf("run not found: %s", runID)
	}

	// Filter events from the specified superstep
	var filtered []ExecutionEvent
	for i := range run.Events {
		if int64(run.Events[i].Superstep) >= fromStep {
			filtered = append(filtered, run.Events[i])
		}
	}

	return filtered, nil
}

// Query retrieves events matching the given filter
//
//nolint:gocyclo // Function handles multiple filter combinations
func (es *EventStore) Query(runID string, filter EventFilter) ([]ExecutionEvent, error) {
	es.mu.RLock()
	defer es.mu.RUnlock()

	run, exists := es.runs[runID]
	if !exists {
		return nil, fmt.Errorf("run not found: %s", runID)
	}

	// Build candidate IDs based on filters
	candidateIDs := es.buildCandidateIDs(run, filter)

	// Build event map for quick lookup
	eventMap := make(map[string]ExecutionEvent)
	for i := range run.Events {
		if candidateIDs[run.Events[i].ID] {
			eventMap[run.Events[i].ID] = run.Events[i]
		}
	}

	// Filter by time range and search text
	filtered := make([]ExecutionEvent, 0, len(candidateIDs))
	for id := range candidateIDs {
		event := eventMap[id]

		// Time filter
		if filter.StartTime != nil && event.Timestamp.Before(*filter.StartTime) {
			continue
		}
		if filter.EndTime != nil && event.Timestamp.After(*filter.EndTime) {
			continue
		}

		// Search text filter
		if filter.SearchText != "" {
			searchLower := strings.ToLower(filter.SearchText)
			if !strings.Contains(strings.ToLower(event.Node), searchLower) &&
				!strings.Contains(strings.ToLower(string(event.Type)), searchLower) &&
				!strings.Contains(strings.ToLower(event.Payload.Error), searchLower) {
				continue
			}
		}

		filtered = append(filtered, event)
	}

	// Sort by timestamp
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Timestamp.Before(filtered[j].Timestamp)
	})

	// Apply pagination
	if filter.Offset > 0 {
		if filter.Offset >= len(filtered) {
			return []ExecutionEvent{}, nil
		}
		filtered = filtered[filter.Offset:]
	}

	if filter.Limit > 0 && filter.Limit < len(filtered) {
		filtered = filtered[:filter.Limit]
	}

	return filtered, nil
}

// GetRun retrieves run metadata
func (es *EventStore) GetRun(runID string) (*RunEvents, error) {
	es.mu.RLock()
	defer es.mu.RUnlock()

	run, exists := es.runs[runID]
	if !exists {
		return nil, fmt.Errorf("run not found: %s", runID)
	}

	// Return a copy to prevent data races during JSON encoding
	runCopy := &RunEvents{
		RunID:     run.RunID,
		GraphID:   run.GraphID,
		Events:    make([]ExecutionEvent, len(run.Events)),
		StartTime: run.StartTime,
		EndTime:   run.EndTime,
		Status:    run.Status,
		Index:     run.Index, // Index is read-only after creation, safe to share
	}
	copy(runCopy.Events, run.Events)

	return runCopy, nil
}

// GetRuns retrieves all runs
func (es *EventStore) GetRuns() []*RunEvents {
	es.mu.RLock()
	defer es.mu.RUnlock()

	runs := make([]*RunEvents, 0, len(es.runs))
	for _, run := range es.runs {
		// Return copies to prevent data races during JSON encoding
		runCopy := &RunEvents{
			RunID:     run.RunID,
			GraphID:   run.GraphID,
			Events:    make([]ExecutionEvent, len(run.Events)),
			StartTime: run.StartTime,
			EndTime:   run.EndTime,
			Status:    run.Status,
			Index:     run.Index, // Index is read-only after creation, safe to share
		}
		copy(runCopy.Events, run.Events)
		runs = append(runs, runCopy)
	}

	return runs
}

// UpdateRunStatus updates the status of a run
func (es *EventStore) UpdateRunStatus(runID string, status RunStatus) error {
	es.mu.Lock()
	defer es.mu.Unlock()

	run, exists := es.runs[runID]
	if !exists {
		return fmt.Errorf("run not found: %s", runID)
	}

	run.Status = status
	if status == StatusCompleted || status == StatusFailed {
		now := time.Now()
		run.EndTime = &now
	}

	return nil
}

// Clear removes all events for a run
func (es *EventStore) Clear(runID string) error {
	es.mu.Lock()
	defer es.mu.Unlock()

	delete(es.runs, runID)
	return nil
}

// buildCandidateIDs builds a set of candidate event IDs based on filters
func (es *EventStore) buildCandidateIDs(run *RunEvents, filter EventFilter) map[string]bool {
	candidateIDs := make(map[string]bool)

	// If type filter is specified, use type index
	if len(filter.Types) > 0 {
		for _, eventType := range filter.Types {
			if ids, ok := run.Index.byType[eventType]; ok {
				for _, id := range ids {
					candidateIDs[id] = true
				}
			}
		}
	} else {
		// No type filter, consider all events
		for i := range run.Events {
			candidateIDs[run.Events[i].ID] = true
		}
	}

	// Filter by node if specified
	if len(filter.Nodes) > 0 {
		es.filterByNodes(run, candidateIDs, filter.Nodes)
	}

	return candidateIDs
}

// filterByNodes filters candidate IDs by node names
func (es *EventStore) filterByNodes(run *RunEvents, candidateIDs map[string]bool, nodes []string) {
	nodeIDs := make(map[string]bool)
	for _, node := range nodes {
		if ids, ok := run.Index.byNode[node]; ok {
			for _, id := range ids {
				nodeIDs[id] = true
			}
		}
	}
	// Intersect with candidate IDs
	for id := range candidateIDs {
		if !nodeIDs[id] {
			delete(candidateIDs, id)
		}
	}
}
