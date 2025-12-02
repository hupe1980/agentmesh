package graph_test

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/stretchr/testify/assert"
)

func TestWithStreamWriter(t *testing.T) {
	ctx := context.Background()

	// Initially no stream writer
	writer := graph.GetStreamWriter(ctx)
	assert.Nil(t, writer)

	// Attach stream writer
	var received graph.Updates
	myWriter := func(updates graph.Updates) {
		received = updates
	}

	ctx = graph.WithStreamWriter(ctx, myWriter)
	writer = graph.GetStreamWriter(ctx)
	assert.NotNil(t, writer)

	// Call the writer
	writer(graph.Updates{"key": "value"})
	assert.Equal(t, "value", received["key"])
}

func TestGetStreamWriterNotSet(t *testing.T) {
	ctx := context.Background()
	writer := graph.GetStreamWriter(ctx)
	assert.Nil(t, writer)
}

func TestStreamWriterMultipleWrites(t *testing.T) {
	var writes []graph.Updates

	writer := func(updates graph.Updates) {
		// Make a copy to avoid issues with map reuse
		copy := make(graph.Updates)
		for k, v := range updates {
			copy[k] = v
		}
		writes = append(writes, copy)
	}

	ctx := graph.WithStreamWriter(context.Background(), writer)
	sw := graph.GetStreamWriter(ctx)

	sw(graph.Updates{"progress": 1})
	sw(graph.Updates{"progress": 2})
	sw(graph.Updates{"progress": 3})

	assert.Len(t, writes, 3)
	assert.Equal(t, 1, writes[0]["progress"])
	assert.Equal(t, 2, writes[1]["progress"])
	assert.Equal(t, 3, writes[2]["progress"])
}

func TestStreamWriterNilSafe(t *testing.T) {
	// Common pattern: check if writer is available before using
	ctx := context.Background()
	sw := graph.GetStreamWriter(ctx)

	// This pattern should be safe
	if sw != nil {
		sw(graph.Updates{"test": "value"})
	}

	// If writer is nil, nothing happens (no panic)
	assert.Nil(t, sw)
}

func TestStreamWriterContextChain(t *testing.T) {
	var received graph.Updates
	writer := func(updates graph.Updates) {
		received = updates
	}

	// Create context chain
	ctx := context.Background()
	ctx = context.WithValue(ctx, "other-key", "other-value")
	ctx = graph.WithStreamWriter(ctx, writer)
	ctx = context.WithValue(ctx, "another-key", "another-value")

	// Should still retrieve writer
	sw := graph.GetStreamWriter(ctx)
	assert.NotNil(t, sw)

	sw(graph.Updates{"nested": true})
	assert.Equal(t, true, received["nested"])
}
