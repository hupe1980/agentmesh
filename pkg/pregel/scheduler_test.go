package pregel

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockTopologyProvider implements TopologyProvider for testing
type mockTopologyProvider struct {
	outgoing map[string][]string
	roots    []string
}

func (m *mockTopologyProvider) Outgoing(vertex string) []string {
	return m.outgoing[vertex]
}

func (m *mockTopologyProvider) RootVertices() []string {
	return m.roots
}

func TestTopologicalScheduler_NextBatch(t *testing.T) {
	scheduler := NewTopologicalScheduler()
	ctx := context.Background()

	t.Run("empty frontier", func(t *testing.T) {
		info := SchedulerInfo{
			Frontier:  make(map[string]struct{}),
			Superstep: 1,
		}

		batch, err := scheduler.NextBatch(ctx, info)
		assert.NoError(t, err)
		assert.Empty(t, batch)
	})

	t.Run("single vertex", func(t *testing.T) {
		info := SchedulerInfo{
			Frontier: map[string]struct{}{
				"vertex_a": {},
			},
			Superstep: 1,
		}

		batch, err := scheduler.NextBatch(ctx, info)
		assert.NoError(t, err)
		assert.Equal(t, []string{"vertex_a"}, batch)
	})

	t.Run("multiple vertices sorted", func(t *testing.T) {
		info := SchedulerInfo{
			Frontier: map[string]struct{}{
				"vertex_c": {},
				"vertex_a": {},
				"vertex_b": {},
			},
			Superstep: 1,
		}

		batch, err := scheduler.NextBatch(ctx, info)
		assert.NoError(t, err)
		assert.Equal(t, []string{"vertex_a", "vertex_b", "vertex_c"}, batch)
	})

	t.Run("deterministic order", func(t *testing.T) {
		// Run multiple times to verify consistent ordering
		info := SchedulerInfo{
			Frontier: map[string]struct{}{
				"z": {},
				"a": {},
				"m": {},
				"b": {},
			},
			Superstep: 1,
		}

		for i := 0; i < 10; i++ {
			batch, err := scheduler.NextBatch(ctx, info)
			assert.NoError(t, err)
			assert.Equal(t, []string{"a", "b", "m", "z"}, batch)
		}
	})

	t.Run("ignores message counts", func(t *testing.T) {
		info := SchedulerInfo{
			Frontier: map[string]struct{}{
				"high_traffic": {},
				"low_traffic":  {},
			},
			Superstep: 1,
			MessageCounts: map[string]int{
				"high_traffic": 1000,
				"low_traffic":  1,
			},
		}

		batch, err := scheduler.NextBatch(ctx, info)
		assert.NoError(t, err)
		// Should sort lexicographically, not by message count
		assert.Equal(t, []string{"high_traffic", "low_traffic"}, batch)
	})

	t.Run("ignores topology", func(t *testing.T) {
		topology := &mockTopologyProvider{
			outgoing: map[string][]string{
				"a": {"b", "c"},
				"b": {"c"},
				"c": {},
			},
			roots: []string{"a"},
		}

		info := SchedulerInfo{
			Frontier: map[string]struct{}{
				"c": {},
				"a": {},
				"b": {},
			},
			Superstep: 1,
			Graph:     topology,
		}

		batch, err := scheduler.NextBatch(ctx, info)
		assert.NoError(t, err)
		// Should sort lexicographically, not topologically
		assert.Equal(t, []string{"a", "b", "c"}, batch)
	})
}

func TestTopologicalScheduler_RecordCompletion(t *testing.T) {
	scheduler := NewTopologicalScheduler()
	ctx := context.Background()

	// RecordCompletion should be a no-op and not panic
	assert.NotPanics(t, func() {
		scheduler.RecordCompletion(ctx, "vertex_a", CompletionInfo{
			Duration:     1000000,
			MessagesSent: 5,
			Error:        nil,
		})
	})

	// Completion shouldn't affect subsequent scheduling
	info := SchedulerInfo{
		Frontier: map[string]struct{}{
			"vertex_b": {},
			"vertex_a": {},
		},
		Superstep: 2,
	}

	batch, err := scheduler.NextBatch(ctx, info)
	assert.NoError(t, err)
	assert.Equal(t, []string{"vertex_a", "vertex_b"}, batch)
}

func TestTopologicalScheduler_ContextCancellation(t *testing.T) {
	scheduler := NewTopologicalScheduler()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	info := SchedulerInfo{
		Frontier: map[string]struct{}{
			"vertex_a": {},
		},
		Superstep: 1,
	}

	// Should still return batch (cancellation checked by runtime, not scheduler)
	batch, err := scheduler.NextBatch(ctx, info)
	assert.NoError(t, err)
	assert.Equal(t, []string{"vertex_a"}, batch)
}

func BenchmarkTopologicalScheduler_NextBatch(b *testing.B) {
	scheduler := NewTopologicalScheduler()
	ctx := context.Background()

	benchmarks := []struct {
		name         string
		frontierSize int
	}{
		{"small_10", 10},
		{"medium_100", 100},
		{"large_1000", 1000},
		{"xlarge_10000", 10000},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			// Create frontier
			frontier := make(map[string]struct{}, bm.frontierSize)
			for i := 0; i < bm.frontierSize; i++ {
				frontier[string(rune('a'+i%26))+string(rune('a'+(i/26)%26))] = struct{}{}
			}

			info := SchedulerInfo{
				Frontier:  frontier,
				Superstep: 1,
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := scheduler.NextBatch(ctx, info)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
