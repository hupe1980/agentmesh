package channel

import (
	"context"
	"sync"
	"testing"
)

func TestTopicChannel(t *testing.T) {
	ctx := context.Background()
	tc := NewTopicChannel("test", 0)

	// Test initial read
	val, err := tc.Read(ctx)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if len(val.([]any)) != 0 {
		t.Errorf("Expected empty slice, got %v", val)
	}

	// Test single write
	if err := tc.Write(ctx, "hello"); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	val, _ = tc.Read(ctx)
	values := val.([]any)
	if len(values) != 1 || values[0] != "hello" {
		t.Errorf("Expected [hello], got %v", values)
	}

	// Test slice write
	if err := tc.Write(ctx, []any{"world", "!"}); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	val, _ = tc.Read(ctx)
	values = val.([]any)
	if len(values) != 3 {
		t.Errorf("Expected 3 values, got %d", len(values))
	}

	// Test version
	if tc.Version() != 2 {
		t.Errorf("Expected version 2, got %d", tc.Version())
	}
}

func TestTopicChannel_MaxValues(t *testing.T) {
	ctx := context.Background()
	tc := NewTopicChannel("test", 2)

	tc.Write(ctx, "a")
	tc.Write(ctx, "b")
	tc.Write(ctx, "c")

	val, _ := tc.Read(ctx)
	values := val.([]any)

	if len(values) != 2 {
		t.Errorf("Expected 2 values (max), got %d", len(values))
	}
	if values[0] != "b" || values[1] != "c" {
		t.Errorf("Expected [b c], got %v", values)
	}
}

func TestLastValueChannel(t *testing.T) {
	ctx := context.Background()
	lvc := NewLastValueChannel("test")

	// Test initial read
	val, err := lvc.Read(ctx)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if val != nil {
		t.Errorf("Expected nil, got %v", val)
	}

	if lvc.HasValue() {
		t.Error("Expected HasValue() = false")
	}

	// Test write and overwrite
	lvc.Write(ctx, "first")
	val, _ = lvc.Read(ctx)
	if val != "first" {
		t.Errorf("Expected 'first', got %v", val)
	}
	if !lvc.HasValue() {
		t.Error("Expected HasValue() = true")
	}

	lvc.Write(ctx, "second")
	val, _ = lvc.Read(ctx)
	if val != "second" {
		t.Errorf("Expected 'second', got %v", val)
	}

	// Test version
	if lvc.Version() != 2 {
		t.Errorf("Expected version 2, got %d", lvc.Version())
	}
}

func TestBinaryOpChannel(t *testing.T) {
	ctx := context.Background()

	// Test with sum operator
	sumOp := func(cur, inc any) any {
		if cur == nil {
			cur = 0
		}
		if inc == nil {
			return cur
		}
		return cur.(int) + inc.(int)
	}

	boc := NewBinaryOpChannel("sum", 0, sumOp)

	// Initial value
	val, _ := boc.Read(ctx)
	if val != 0 {
		t.Errorf("Expected 0, got %v", val)
	}

	// Write values
	boc.Write(ctx, 10)
	boc.Write(ctx, 20)
	boc.Write(ctx, 30)

	val, _ = boc.Read(ctx)
	if val != 60 {
		t.Errorf("Expected 60, got %v", val)
	}

	// Test version
	if boc.Version() != 3 {
		t.Errorf("Expected version 3, got %d", boc.Version())
	}
}

func TestBinaryOpChannel_MapMerge(t *testing.T) {
	ctx := context.Background()

	// Test with map merge operator
	mergeOp := func(cur, inc any) any {
		if cur == nil {
			cur = make(map[string]any)
		}
		if inc == nil {
			return cur
		}

		result := make(map[string]any)
		for k, v := range cur.(map[string]any) {
			result[k] = v
		}
		for k, v := range inc.(map[string]any) {
			result[k] = v
		}
		return result
	}

	boc := NewBinaryOpChannel("state", make(map[string]any), mergeOp)

	boc.Write(ctx, map[string]any{"a": 1, "b": 2})
	boc.Write(ctx, map[string]any{"b": 3, "c": 4})

	val, _ := boc.Read(ctx)
	m := val.(map[string]any)

	if len(m) != 3 {
		t.Errorf("Expected 3 keys, got %d", len(m))
	}
	if m["a"] != 1 || m["b"] != 3 || m["c"] != 4 {
		t.Errorf("Unexpected map values: %v", m)
	}
}

func TestChannelSet(t *testing.T) {
	ctx := context.Background()
	cs := NewChannelSet()

	// Add channels
	tc := NewTopicChannel("messages", 0)
	lvc := NewLastValueChannel("context")

	cs.Add(tc)
	cs.Add(lvc)

	// Test Get
	ch, ok := cs.Get("messages")
	if !ok || ch.Name() != "messages" {
		t.Error("Failed to get 'messages' channel")
	}

	// Test List
	names := cs.List()
	if len(names) != 2 {
		t.Errorf("Expected 2 channels, got %d", len(names))
	}

	// Test WriteAll
	updates := map[string]any{
		"messages": "hello",
		"context":  map[string]any{"user": "alice"},
		"unknown":  "ignored", // Should be ignored
	}

	if err := cs.WriteAll(ctx, updates); err != nil {
		t.Fatalf("WriteAll() error = %v", err)
	}

	// Test ReadAll
	values, err := cs.ReadAll(ctx)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}

	if len(values) != 2 {
		t.Errorf("Expected 2 values, got %d", len(values))
	}

	msgs := values["messages"].([]any)
	if len(msgs) != 1 || msgs[0] != "hello" {
		t.Errorf("Unexpected messages: %v", msgs)
	}
}

func TestChannelConcurrency(t *testing.T) {
	ctx := context.Background()
	tc := NewTopicChannel("concurrent", 0)

	// Concurrent writes
	var wg sync.WaitGroup
	writers := 100
	wg.Add(writers)

	for i := 0; i < writers; i++ {
		go func(val int) {
			defer wg.Done()
			tc.Write(ctx, val)
		}(i)
	}

	wg.Wait()

	val, _ := tc.Read(ctx)
	values := val.([]any)
	if len(values) != writers {
		t.Errorf("Expected %d values, got %d", writers, len(values))
	}
}

func TestChannelReset(t *testing.T) {
	ctx := context.Background()

	t.Run("TopicChannel", func(t *testing.T) {
		tc := NewTopicChannel("test", 0)
		tc.Write(ctx, "value")

		if err := tc.Reset(ctx); err != nil {
			t.Fatalf("Reset() error = %v", err)
		}

		val, _ := tc.Read(ctx)
		if len(val.([]any)) != 0 {
			t.Error("Expected empty after reset")
		}
		if tc.Version() != 0 {
			t.Errorf("Expected version 0 after reset, got %d", tc.Version())
		}
	})

	t.Run("LastValueChannel", func(t *testing.T) {
		lvc := NewLastValueChannel("test")
		lvc.Write(ctx, "value")

		if err := lvc.Reset(ctx); err != nil {
			t.Fatalf("Reset() error = %v", err)
		}

		if lvc.HasValue() {
			t.Error("Expected HasValue() = false after reset")
		}
		if lvc.Version() != 0 {
			t.Errorf("Expected version 0 after reset, got %d", lvc.Version())
		}
	})
}

func TestChannelSnapshot(t *testing.T) {
	ctx := context.Background()
	tc := NewTopicChannel("test", 0)

	tc.Write(ctx, "a")
	tc.Write(ctx, "b")

	snap, err := tc.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}

	values := snap.([]any)
	if len(values) != 2 {
		t.Errorf("Expected 2 values in snapshot, got %d", len(values))
	}

	// Verify snapshot is independent
	tc.Write(ctx, "c")

	// Original snapshot should still have 2 values
	if len(values) != 2 {
		t.Error("Snapshot was not independent")
	}
}
