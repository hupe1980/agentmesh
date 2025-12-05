package pregel

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPriorityScheduler_NextBatch(t *testing.T) {
	t.Run("empty frontier", func(t *testing.T) {
		scheduler := NewPriorityScheduler(nil, 50)
		info := SchedulerInfo{
			Frontier:  make(map[string]struct{}),
			Superstep: 1,
		}

		batch, err := scheduler.NextBatch(context.Background(), info)
		assert.NoError(t, err)
		assert.Empty(t, batch)
	})

	t.Run("priority ordering", func(t *testing.T) {
		priorities := map[string]int{
			"high":   100,
			"medium": 50,
			"low":    10,
		}
		scheduler := NewPriorityScheduler(priorities, 50)

		info := SchedulerInfo{
			Frontier: map[string]struct{}{
				"low":    {},
				"high":   {},
				"medium": {},
			},
			Superstep: 1,
		}

		batch, err := scheduler.NextBatch(context.Background(), info)
		assert.NoError(t, err)
		assert.Equal(t, []string{"high", "medium", "low"}, batch)
	})

	t.Run("equal priority sorted lexicographically", func(t *testing.T) {
		priorities := map[string]int{
			"zebra": 50,
			"apple": 50,
			"mango": 50,
		}
		scheduler := NewPriorityScheduler(priorities, 50)

		info := SchedulerInfo{
			Frontier: map[string]struct{}{
				"zebra": {},
				"apple": {},
				"mango": {},
			},
			Superstep: 1,
		}

		batch, err := scheduler.NextBatch(context.Background(), info)
		assert.NoError(t, err)
		assert.Equal(t, []string{"apple", "mango", "zebra"}, batch)
	})

	t.Run("default priority for unknown vertices", func(t *testing.T) {
		priorities := map[string]int{
			"known": 100,
		}
		scheduler := NewPriorityScheduler(priorities, 50)

		info := SchedulerInfo{
			Frontier: map[string]struct{}{
				"known":   {},
				"unknown": {},
			},
			Superstep: 1,
		}

		batch, err := scheduler.NextBatch(context.Background(), info)
		assert.NoError(t, err)
		// "known" (100) > "unknown" (50 default)
		assert.Equal(t, []string{"known", "unknown"}, batch)
	})

	t.Run("dynamic priority updates", func(t *testing.T) {
		scheduler := NewPriorityScheduler(nil, 50)

		// Set priorities dynamically
		scheduler.SetPriority("urgent", 200)
		scheduler.SetPriority("normal", 100)

		info := SchedulerInfo{
			Frontier: map[string]struct{}{
				"normal": {},
				"urgent": {},
			},
			Superstep: 1,
		}

		batch, err := scheduler.NextBatch(context.Background(), info)
		assert.NoError(t, err)
		assert.Equal(t, []string{"urgent", "normal"}, batch)

		// Update priority
		scheduler.SetPriority("normal", 300)

		batch, err = scheduler.NextBatch(context.Background(), info)
		assert.NoError(t, err)
		assert.Equal(t, []string{"normal", "urgent"}, batch)
	})
}

func TestPriorityScheduler_GetSetPriority(t *testing.T) {
	scheduler := NewPriorityScheduler(nil, 50)

	// Test default
	assert.Equal(t, 50, scheduler.GetPriority("unknown"))

	// Test set and get
	scheduler.SetPriority("test", 100)
	assert.Equal(t, 100, scheduler.GetPriority("test"))

	// Test overwrite
	scheduler.SetPriority("test", 200)
	assert.Equal(t, 200, scheduler.GetPriority("test"))
}

func TestPriorityScheduler_RecordCompletion(t *testing.T) {
	scheduler := NewPriorityScheduler(nil, 50)

	// Should not panic (no-op)
	assert.NotPanics(t, func() {
		scheduler.RecordCompletion(context.Background(), "vertex", CompletionInfo{
			Duration:     1000000,
			MessagesSent: 5,
			Error:        nil,
		})
	})
}

func TestResourceAwareScheduler_NextBatch(t *testing.T) {
	t.Run("empty frontier", func(t *testing.T) {
		scheduler := NewResourceAwareScheduler(nil, 50, true)
		info := SchedulerInfo{
			Frontier:  make(map[string]struct{}),
			Superstep: 1,
		}

		batch, err := scheduler.NextBatch(context.Background(), info)
		assert.NoError(t, err)
		assert.Empty(t, batch)
	})

	t.Run("low cost first", func(t *testing.T) {
		costs := map[string]int{
			"expensive": 100,
			"medium":    50,
			"cheap":     10,
		}
		scheduler := NewResourceAwareScheduler(costs, 50, true)

		info := SchedulerInfo{
			Frontier: map[string]struct{}{
				"expensive": {},
				"cheap":     {},
				"medium":    {},
			},
			Superstep: 1,
		}

		batch, err := scheduler.NextBatch(context.Background(), info)
		assert.NoError(t, err)
		assert.Equal(t, []string{"cheap", "medium", "expensive"}, batch)
	})

	t.Run("high cost first", func(t *testing.T) {
		costs := map[string]int{
			"expensive": 100,
			"medium":    50,
			"cheap":     10,
		}
		scheduler := NewResourceAwareScheduler(costs, 50, false)

		info := SchedulerInfo{
			Frontier: map[string]struct{}{
				"expensive": {},
				"cheap":     {},
				"medium":    {},
			},
			Superstep: 1,
		}

		batch, err := scheduler.NextBatch(context.Background(), info)
		assert.NoError(t, err)
		assert.Equal(t, []string{"expensive", "medium", "cheap"}, batch)
	})

	t.Run("equal cost sorted lexicographically", func(t *testing.T) {
		costs := map[string]int{
			"zebra": 50,
			"apple": 50,
			"mango": 50,
		}
		scheduler := NewResourceAwareScheduler(costs, 50, true)

		info := SchedulerInfo{
			Frontier: map[string]struct{}{
				"zebra": {},
				"apple": {},
				"mango": {},
			},
			Superstep: 1,
		}

		batch, err := scheduler.NextBatch(context.Background(), info)
		assert.NoError(t, err)
		assert.Equal(t, []string{"apple", "mango", "zebra"}, batch)
	})

	t.Run("default cost for unknown vertices", func(t *testing.T) {
		costs := map[string]int{
			"known": 10,
		}
		scheduler := NewResourceAwareScheduler(costs, 50, true)

		info := SchedulerInfo{
			Frontier: map[string]struct{}{
				"known":   {},
				"unknown": {},
			},
			Superstep: 1,
		}

		batch, err := scheduler.NextBatch(context.Background(), info)
		assert.NoError(t, err)
		// "known" (10) < "unknown" (50 default)
		assert.Equal(t, []string{"known", "unknown"}, batch)
	})

	t.Run("dynamic cost updates", func(t *testing.T) {
		scheduler := NewResourceAwareScheduler(nil, 50, true)

		scheduler.SetResourceCost("heavy", 100)
		scheduler.SetResourceCost("light", 10)

		info := SchedulerInfo{
			Frontier: map[string]struct{}{
				"heavy": {},
				"light": {},
			},
			Superstep: 1,
		}

		batch, err := scheduler.NextBatch(context.Background(), info)
		assert.NoError(t, err)
		assert.Equal(t, []string{"light", "heavy"}, batch)

		// Swap costs
		scheduler.SetResourceCost("heavy", 5)
		scheduler.SetResourceCost("light", 200)

		batch, err = scheduler.NextBatch(context.Background(), info)
		assert.NoError(t, err)
		assert.Equal(t, []string{"heavy", "light"}, batch)
	})
}

func TestResourceAwareScheduler_GetSetResourceCost(t *testing.T) {
	scheduler := NewResourceAwareScheduler(nil, 50, true)

	// Test default
	assert.Equal(t, 50, scheduler.GetResourceCost("unknown"))

	// Test set and get
	scheduler.SetResourceCost("test", 100)
	assert.Equal(t, 100, scheduler.GetResourceCost("test"))

	// Test overwrite
	scheduler.SetResourceCost("test", 200)
	assert.Equal(t, 200, scheduler.GetResourceCost("test"))
}

func TestResourceAwareScheduler_RecordCompletion(t *testing.T) {
	scheduler := NewResourceAwareScheduler(nil, 50, true)

	// Should not panic (no-op)
	assert.NotPanics(t, func() {
		scheduler.RecordCompletion(context.Background(), "vertex", CompletionInfo{
			Duration:     1000000,
			MessagesSent: 5,
			Error:        nil,
		})
	})
}

func BenchmarkPriorityScheduler_NextBatch(b *testing.B) {
	priorities := make(map[string]int, 1000)
	frontier := make(map[string]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		vertex := string(rune('a' + i%26))
		priorities[vertex] = i % 100
		frontier[vertex] = struct{}{}
	}

	scheduler := NewPriorityScheduler(priorities, 50)
	ctx := context.Background()
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
}

func BenchmarkResourceAwareScheduler_NextBatch(b *testing.B) {
	costs := make(map[string]int, 1000)
	frontier := make(map[string]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		vertex := string(rune('a' + i%26))
		costs[vertex] = i % 100
		frontier[vertex] = struct{}{}
	}

	scheduler := NewResourceAwareScheduler(costs, 50, true)
	ctx := context.Background()
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
}
