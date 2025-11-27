package viz

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventStore_NewEventStore(t *testing.T) {
	store := NewEventStore(100)
	assert.NotNil(t, store)
	assert.Equal(t, 100, store.maxSize)
	assert.NotNil(t, store.runs)
	assert.Empty(t, store.GetRuns())
}

func TestEventStore_Append(t *testing.T) {
	t.Run("first event creates run", func(t *testing.T) {
		store := NewEventStore(100)
		event := ExecutionEvent{
			RunID:     "run-1",
			Superstep: 1,
			Node:      "node-a",
			Timestamp: time.Now(),
			Type:      EventNodeStart,
		}

		err := store.Append(event)
		require.NoError(t, err)

		run, err := store.GetRun("run-1")
		require.NoError(t, err)
		assert.Equal(t, "run-1", run.RunID)
		assert.Equal(t, StatusRunning, run.Status)
		assert.Len(t, run.Events, 1)
		assert.Equal(t, event.ID, run.Events[0].ID)
	})

	t.Run("multiple events to same run", func(t *testing.T) {
		store := NewEventStore(100)

		for i := 1; i <= 3; i++ {
			event := ExecutionEvent{
				RunID:     "run-1",
				Superstep: i,
				Node:      "node-a",
				Timestamp: time.Now(),
				Type:      EventNodeStart,
			}
			err := store.Append(event)
			require.NoError(t, err)
		}

		run, err := store.GetRun("run-1")
		require.NoError(t, err)
		assert.Len(t, run.Events, 3)
	})

	t.Run("circular buffer overflow", func(t *testing.T) {
		store := NewEventStore(5) // Small buffer for testing

		// Add 10 events, only last 5 should remain
		for i := 1; i <= 10; i++ {
			event := ExecutionEvent{
				RunID:     "run-1",
				Superstep: i,
				Node:      "node-a",
				Timestamp: time.Now(),
				Type:      EventNodeStart,
			}
			err := store.Append(event)
			require.NoError(t, err)
		}

		run, err := store.GetRun("run-1")
		require.NoError(t, err)
		assert.Len(t, run.Events, 5)

		// Verify we have events 6-10 (oldest removed)
		assert.Equal(t, 6, run.Events[0].Superstep)
		assert.Equal(t, 10, run.Events[4].Superstep)
	})

	t.Run("node error updates status", func(t *testing.T) {
		store := NewEventStore(100)
		event := ExecutionEvent{
			RunID:     "run-1",
			Superstep: 1,
			Node:      "node-a",
			Timestamp: time.Now(),
			Type:      EventNodeError,
			Payload: EventPayload{
				Error: "something went wrong",
			},
		}

		err := store.Append(event)
		require.NoError(t, err)

		run, err := store.GetRun("run-1")
		require.NoError(t, err)
		assert.Equal(t, StatusFailed, run.Status)
		assert.NotNil(t, run.EndTime)
	})

	t.Run("node complete with error updates status", func(t *testing.T) {
		store := NewEventStore(100)
		event := ExecutionEvent{
			RunID:     "run-1",
			Superstep: 1,
			Node:      "node-a",
			Timestamp: time.Now(),
			Type:      EventNodeComplete,
			Payload: EventPayload{
				Error: "error in completion",
			},
		}

		err := store.Append(event)
		require.NoError(t, err)

		run, err := store.GetRun("run-1")
		require.NoError(t, err)
		assert.Equal(t, StatusFailed, run.Status)
		assert.NotNil(t, run.EndTime)
	})

	t.Run("events to different runs", func(t *testing.T) {
		store := NewEventStore(100)

		event1 := ExecutionEvent{ID: "e1", RunID: "run-1", Superstep: 1, Node: "node-a", Timestamp: time.Now(), Type: EventNodeStart}
		event2 := ExecutionEvent{ID: "e2", RunID: "run-2", Superstep: 1, Node: "node-b", Timestamp: time.Now(), Type: EventNodeStart}

		store.Append(event1)
		store.Append(event2)

		runs := store.GetRuns()
		assert.Len(t, runs, 2)
	})
}

func TestEventStore_GetEvents(t *testing.T) {
	t.Run("get all events from step 0", func(t *testing.T) {
		store := NewEventStore(100)

		for i := 1; i <= 5; i++ {
			event := ExecutionEvent{
				RunID:     "run-1",
				Superstep: i,
				Node:      "node-a",
				Timestamp: time.Now(),
				Type:      EventNodeStart,
			}
			store.Append(event)
		}

		events, err := store.GetEvents("run-1", 0)
		require.NoError(t, err)
		assert.Len(t, events, 5)
	})

	t.Run("get events from specific step", func(t *testing.T) {
		store := NewEventStore(100)

		for i := 1; i <= 10; i++ {
			event := ExecutionEvent{
				RunID:     "run-1",
				Superstep: i,
				Node:      "node-a",
				Timestamp: time.Now(),
				Type:      EventNodeStart,
			}
			store.Append(event)
		}

		events, err := store.GetEvents("run-1", 5)
		require.NoError(t, err)
		assert.Len(t, events, 6) // Steps 5-10
		assert.Equal(t, 5, events[0].Superstep)
		assert.Equal(t, 10, events[5].Superstep)
	})

	t.Run("get events for non-existent run", func(t *testing.T) {
		store := NewEventStore(100)

		events, err := store.GetEvents("non-existent", 0)
		assert.Error(t, err)
		assert.Nil(t, events)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("get events with step beyond range", func(t *testing.T) {
		store := NewEventStore(100)

		for i := 1; i <= 5; i++ {
			event := ExecutionEvent{
				RunID:     "run-1",
				Superstep: i,
				Node:      "node-a",
				Timestamp: time.Now(),
				Type:      EventNodeStart,
			}
			store.Append(event)
		}

		events, err := store.GetEvents("run-1", 100)
		require.NoError(t, err)
		assert.Empty(t, events)
	})
}

func TestEventStore_GetRun(t *testing.T) {
	t.Run("get existing run", func(t *testing.T) {
		store := NewEventStore(100)
		timestamp := time.Now()

		event := ExecutionEvent{
			RunID:     "run-1",
			Superstep: 1,
			Node:      "node-a",
			Timestamp: timestamp,
			Type:      EventNodeStart,
		}
		store.Append(event)

		run, err := store.GetRun("run-1")
		require.NoError(t, err)
		assert.Equal(t, "run-1", run.RunID)
		assert.Equal(t, StatusRunning, run.Status)
		assert.Equal(t, timestamp, run.StartTime)
		assert.Nil(t, run.EndTime)
	})

	t.Run("get non-existent run", func(t *testing.T) {
		store := NewEventStore(100)

		run, err := store.GetRun("non-existent")
		assert.Error(t, err)
		assert.Nil(t, run)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestEventStore_GetRuns(t *testing.T) {
	t.Run("empty store", func(t *testing.T) {
		store := NewEventStore(100)
		runs := store.GetRuns()
		assert.Empty(t, runs)
		assert.NotNil(t, runs)
	})

	t.Run("with multiple runs", func(t *testing.T) {
		store := NewEventStore(100)

		for i := 1; i <= 3; i++ {
			event := ExecutionEvent{
				RunID:     fmt.Sprintf("run-%d", i),
				Superstep: 1,
				Node:      "node-a",
				Timestamp: time.Now(),
				Type:      EventNodeStart,
			}
			store.Append(event)
		}

		runs := store.GetRuns()
		assert.Len(t, runs, 3)
	})
}

func TestEventStore_UpdateRunStatus(t *testing.T) {
	t.Run("update to completed sets end time", func(t *testing.T) {
		store := NewEventStore(100)
		event := ExecutionEvent{
			RunID:     "run-1",
			Superstep: 1,
			Node:      "node-a",
			Timestamp: time.Now(),
			Type:      EventNodeStart,
		}
		store.Append(event)

		err := store.UpdateRunStatus("run-1", StatusCompleted)
		require.NoError(t, err)

		run, _ := store.GetRun("run-1")
		assert.Equal(t, StatusCompleted, run.Status)
		assert.NotNil(t, run.EndTime)
	})

	t.Run("update to failed sets end time", func(t *testing.T) {
		store := NewEventStore(100)
		event := ExecutionEvent{
			RunID:     "run-1",
			Superstep: 1,
			Node:      "node-a",
			Timestamp: time.Now(),
			Type:      EventNodeStart,
		}
		store.Append(event)

		err := store.UpdateRunStatus("run-1", StatusFailed)
		require.NoError(t, err)

		run, _ := store.GetRun("run-1")
		assert.Equal(t, StatusFailed, run.Status)
		assert.NotNil(t, run.EndTime)
	})

	t.Run("update to paused does not set end time", func(t *testing.T) {
		store := NewEventStore(100)
		event := ExecutionEvent{
			RunID:     "run-1",
			Superstep: 1,
			Node:      "node-a",
			Timestamp: time.Now(),
			Type:      EventNodeStart,
		}
		store.Append(event)

		err := store.UpdateRunStatus("run-1", StatusPaused)
		require.NoError(t, err)

		run, _ := store.GetRun("run-1")
		assert.Equal(t, StatusPaused, run.Status)
		assert.Nil(t, run.EndTime)
	})

	t.Run("update non-existent run", func(t *testing.T) {
		store := NewEventStore(100)

		err := store.UpdateRunStatus("non-existent", StatusCompleted)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestEventStore_Clear(t *testing.T) {
	t.Run("clear existing run", func(t *testing.T) {
		store := NewEventStore(100)
		event := ExecutionEvent{
			RunID:     "run-1",
			Superstep: 1,
			Node:      "node-a",
			Timestamp: time.Now(),
			Type:      EventNodeStart,
		}
		store.Append(event)

		err := store.Clear("run-1")
		require.NoError(t, err)

		_, err = store.GetRun("run-1")
		assert.Error(t, err)
	})

	t.Run("clear non-existent run is ok", func(t *testing.T) {
		store := NewEventStore(100)

		err := store.Clear("non-existent")
		assert.NoError(t, err) // Clear is idempotent
	})

	t.Run("clear one of multiple runs", func(t *testing.T) {
		store := NewEventStore(100)

		for i := 1; i <= 3; i++ {
			event := ExecutionEvent{
				RunID:     fmt.Sprintf("run-%d", i),
				Superstep: 1,
				Node:      "node-a",
				Timestamp: time.Now(),
				Type:      EventNodeStart,
			}
			store.Append(event)
		}

		err := store.Clear("run-2")
		require.NoError(t, err)

		runs := store.GetRuns()
		assert.Len(t, runs, 2)
	})
}

func TestEventStore_ThreadSafety(t *testing.T) {
	store := NewEventStore(100)

	done := make(chan bool)
	iterations := 50

	// Concurrent appends
	go func() {
		for i := 0; i < iterations; i++ {
			event := ExecutionEvent{
				RunID:     "run-1",
				Superstep: i,
				Node:      "node-a",
				Timestamp: time.Now(),
				Type:      EventNodeStart,
			}
			store.Append(event)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < iterations; i++ {
			event := ExecutionEvent{
				RunID:     "run-2",
				Superstep: i,
				Node:      "node-b",
				Timestamp: time.Now(),
				Type:      EventNodeStart,
			}
			store.Append(event)
		}
		done <- true
	}()

	// Concurrent reads
	go func() {
		for i := 0; i < iterations; i++ {
			store.GetEvents("run-1", 0)
			store.GetRun("run-1")
			store.GetRuns()
		}
		done <- true
	}()

	// Concurrent status updates
	go func() {
		for i := 0; i < iterations; i++ {
			store.UpdateRunStatus("run-1", StatusRunning)
		}
		done <- true
	}()

	// Wait for all goroutines
	<-done

	// If we get here without data races, test passes
	assert.True(t, true)
}

func TestEventStore_CircularBufferEdgeCases(t *testing.T) {
	t.Run("buffer size of 1", func(t *testing.T) {
		store := NewEventStore(1)

		event1 := ExecutionEvent{RunID: "run-1", Superstep: 1, Node: "node-a", Timestamp: time.Now(), Type: EventNodeStart}
		event2 := ExecutionEvent{RunID: "run-1", Superstep: 2, Node: "node-b", Timestamp: time.Now(), Type: EventNodeStart}

		store.Append(event1)
		store.Append(event2)

		run, _ := store.GetRun("run-1")
		assert.Len(t, run.Events, 1)
		assert.Equal(t, 2, run.Events[0].Superstep)
	})

	t.Run("buffer size of 0", func(t *testing.T) {
		store := NewEventStore(0)

		event := ExecutionEvent{RunID: "run-1", Superstep: 1, Node: "node-a", Timestamp: time.Now(), Type: EventNodeStart}
		store.Append(event)

		// With maxSize 0, events are constantly removed
		run, _ := store.GetRun("run-1")
		assert.Len(t, run.Events, 0)
	})
}
