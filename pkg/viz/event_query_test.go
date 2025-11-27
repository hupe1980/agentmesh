package viz

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventStore_Query_ByType(t *testing.T) {
	store := NewEventStore(100)
	runID := "test-run-1"

	// Add events of different types
	events := []ExecutionEvent{
		{ID: "e1", RunID: runID, Type: EventNodeStart, Node: "node1", Timestamp: time.Now()},
		{ID: "e2", RunID: runID, Type: EventNodeComplete, Node: "node1", Timestamp: time.Now()},
		{ID: "e3", RunID: runID, Type: EventNodeStart, Node: "node2", Timestamp: time.Now()},
		{ID: "e4", RunID: runID, Type: EventStateUpdate, Node: "node1", Timestamp: time.Now()},
	}

	for _, event := range events {
		err := store.Append(event)
		require.NoError(t, err)
	}

	// Query by type
	filter := EventFilter{
		Types: []EventType{EventNodeStart},
	}
	results, err := store.Query(runID, filter)
	require.NoError(t, err)
	assert.Len(t, results, 2)

	// Verify correct events returned
	for _, event := range results {
		assert.Equal(t, EventNodeStart, event.Type)
	}
}

func TestEventStore_Query_ByNode(t *testing.T) {
	store := NewEventStore(100)
	runID := "test-run-1"

	// Add events for different nodes
	events := []ExecutionEvent{
		{ID: "e1", RunID: runID, Type: EventNodeStart, Node: "node1", Timestamp: time.Now()},
		{ID: "e2", RunID: runID, Type: EventNodeComplete, Node: "node2", Timestamp: time.Now()},
		{ID: "e3", RunID: runID, Type: EventNodeStart, Node: "node1", Timestamp: time.Now()},
	}

	for _, event := range events {
		err := store.Append(event)
		require.NoError(t, err)
	}

	// Query by node
	filter := EventFilter{
		Nodes: []string{"node1"},
	}
	results, err := store.Query(runID, filter)
	require.NoError(t, err)
	assert.Len(t, results, 2)

	// Verify correct events returned
	for _, event := range results {
		assert.Equal(t, "node1", event.Node)
	}
}

func TestEventStore_Query_ByTimeRange(t *testing.T) {
	store := NewEventStore(100)
	runID := "test-run-1"

	now := time.Now()
	past := now.Add(-1 * time.Hour)
	future := now.Add(1 * time.Hour)

	// Add events at different times
	events := []ExecutionEvent{
		{ID: "e1", RunID: runID, Type: EventNodeStart, Node: "node1", Timestamp: past},
		{ID: "e2", RunID: runID, Type: EventNodeComplete, Node: "node1", Timestamp: now},
		{ID: "e3", RunID: runID, Type: EventNodeStart, Node: "node2", Timestamp: future},
	}

	for _, event := range events {
		err := store.Append(event)
		require.NoError(t, err)
	}

	// Query by time range
	startTime := past.Add(30 * time.Minute)
	filter := EventFilter{
		StartTime: &startTime,
	}
	results, err := store.Query(runID, filter)
	require.NoError(t, err)
	assert.Len(t, results, 2) // now and future events

	// Verify correct events returned
	for _, event := range results {
		assert.True(t, event.Timestamp.After(past) || event.Timestamp.Equal(now))
	}
}

func TestEventStore_Query_BySearchText(t *testing.T) {
	store := NewEventStore(100)
	runID := "test-run-1"

	// Add events with different content
	events := []ExecutionEvent{
		{ID: "e1", RunID: runID, Type: EventNodeStart, Node: "analyzer", Timestamp: time.Now()},
		{ID: "e2", RunID: runID, Type: EventNodeComplete, Node: "processor", Timestamp: time.Now()},
		{ID: "e3", RunID: runID, Type: EventNodeError, Node: "validator", Timestamp: time.Now(),
			Payload: EventPayload{Error: "validation failed"}},
	}

	for _, event := range events {
		err := store.Append(event)
		require.NoError(t, err)
	}

	// Query by search text
	filter := EventFilter{
		SearchText: "valid",
	}
	results, err := store.Query(runID, filter)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "e3", results[0].ID)
}

func TestEventStore_Query_Pagination(t *testing.T) {
	store := NewEventStore(100)
	runID := "test-run-1"

	// Add 10 events
	for i := 0; i < 10; i++ {
		event := ExecutionEvent{
			ID:        generateEventID(),
			RunID:     runID,
			Type:      EventNodeStart,
			Node:      "node",
			Timestamp: time.Now(),
			Superstep: i,
		}
		err := store.Append(event)
		require.NoError(t, err)
	}

	// Query with limit
	filter := EventFilter{
		Limit: 5,
	}
	results, err := store.Query(runID, filter)
	require.NoError(t, err)
	assert.Len(t, results, 5)

	// Query with offset
	filter = EventFilter{
		Offset: 5,
	}
	results, err = store.Query(runID, filter)
	require.NoError(t, err)
	assert.Len(t, results, 5)

	// Query with limit and offset
	filter = EventFilter{
		Limit:  3,
		Offset: 2,
	}
	results, err = store.Query(runID, filter)
	require.NoError(t, err)
	assert.Len(t, results, 3)
}

func TestEventStore_Query_Combined(t *testing.T) {
	store := NewEventStore(100)
	runID := "test-run-1"

	// Add various events
	events := []ExecutionEvent{
		{ID: "e1", RunID: runID, Type: EventNodeStart, Node: "node1", Timestamp: time.Now()},
		{ID: "e2", RunID: runID, Type: EventNodeStart, Node: "node2", Timestamp: time.Now()},
		{ID: "e3", RunID: runID, Type: EventNodeComplete, Node: "node1", Timestamp: time.Now()},
		{ID: "e4", RunID: runID, Type: EventNodeStart, Node: "node1", Timestamp: time.Now()},
	}

	for _, event := range events {
		err := store.Append(event)
		require.NoError(t, err)
	}

	// Query with multiple filters
	filter := EventFilter{
		Types: []EventType{EventNodeStart},
		Nodes: []string{"node1"},
	}
	results, err := store.Query(runID, filter)
	require.NoError(t, err)
	assert.Len(t, results, 2) // e1 and e4

	// Verify results
	for _, event := range results {
		assert.Equal(t, EventNodeStart, event.Type)
		assert.Equal(t, "node1", event.Node)
	}
}

func TestEventStore_Query_NonExistentRun(t *testing.T) {
	store := NewEventStore(100)

	filter := EventFilter{
		Types: []EventType{EventNodeStart},
	}
	results, err := store.Query("non-existent", filter)
	assert.Error(t, err)
	assert.Nil(t, results)
	assert.Contains(t, err.Error(), "not found")
}

func TestEventStore_Query_EmptyFilter(t *testing.T) {
	store := NewEventStore(100)
	runID := "test-run-1"

	// Add events
	for i := 0; i < 5; i++ {
		event := ExecutionEvent{
			ID:        generateEventID(),
			RunID:     runID,
			Type:      EventNodeStart,
			Node:      "node",
			Timestamp: time.Now(),
		}
		err := store.Append(event)
		require.NoError(t, err)
	}

	// Query with empty filter (should return all events)
	filter := EventFilter{}
	results, err := store.Query(runID, filter)
	require.NoError(t, err)
	assert.Len(t, results, 5)
}

func TestEventIndex_AddToIndex(t *testing.T) {
	idx := newEventIndex()

	event := ExecutionEvent{
		ID:        "event-1",
		Type:      EventNodeStart,
		Node:      "test-node",
		Timestamp: time.Now(),
	}

	idx.addToIndex(event)

	// Verify type index
	assert.Contains(t, idx.byType, EventNodeStart)
	assert.Contains(t, idx.byType[EventNodeStart], "event-1")

	// Verify node index
	assert.Contains(t, idx.byNode, "test-node")
	assert.Contains(t, idx.byNode["test-node"], "event-1")

	// Verify time index
	assert.Contains(t, idx.byTime, "event-1")
}

func TestEventIndex_MultipleEvents(t *testing.T) {
	idx := newEventIndex()

	events := []ExecutionEvent{
		{ID: "e1", Type: EventNodeStart, Node: "node1", Timestamp: time.Now()},
		{ID: "e2", Type: EventNodeStart, Node: "node2", Timestamp: time.Now()},
		{ID: "e3", Type: EventNodeComplete, Node: "node1", Timestamp: time.Now()},
	}

	for _, event := range events {
		idx.addToIndex(event)
	}

	// Verify type indexes
	assert.Len(t, idx.byType[EventNodeStart], 2)
	assert.Len(t, idx.byType[EventNodeComplete], 1)

	// Verify node indexes
	assert.Len(t, idx.byNode["node1"], 2)
	assert.Len(t, idx.byNode["node2"], 1)

	// Verify time index
	assert.Len(t, idx.byTime, 3)
}
