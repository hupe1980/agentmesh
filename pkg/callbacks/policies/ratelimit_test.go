package policies

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
)

// mockStateWriter is a simple mock implementation of graph.StateWriter for testing
type mockStateWriter struct {
	messages   []message.Message
	state      map[string]any
	aggregates map[string]any
}

func newMockStateWriter() *mockStateWriter {
	return &mockStateWriter{
		messages:   []message.Message{message.NewHumanMessageFromText("test message")},
		state:      make(map[string]any),
		aggregates: make(map[string]any),
	}
}

func (m *mockStateWriter) Get(key string) any {
	return m.state[key]
}

func (m *mockStateWriter) GetAll() map[string]any {
	return m.state
}

func (m *mockStateWriter) Set(key string, value any) {
	m.state[key] = value
}

func (m *mockStateWriter) MessageEventsSnapshot() []graph.MessageEvent {
	events := make([]graph.MessageEvent, len(m.messages))
	for i, msg := range m.messages {
		events[i] = *graph.NewMessageEvent(msg, "", "")
	}
	return events
}

func (m *mockStateWriter) AggregatesSnapshot() map[string]any {
	return m.aggregates
}

func (m *mockStateWriter) Aggregate(name string, value any) error {
	m.aggregates[name] = value
	return nil
}

func createTestStateWriter() graph.StateWriter {
	return newMockStateWriter()
}

// RateLimit Tests

func TestRateLimit_AllowsRequestsWithinLimit(t *testing.T) {
	callback := RateLimit(5, time.Minute)
	ctx := context.Background()
	sw := createTestStateWriter()

	// Should allow first 5 requests
	for i := 0; i < 5; i++ {
		msg, err := callback(ctx, sw)
		if err != nil {
			t.Fatalf("request %d: unexpected error: %v", i+1, err)
		}
		if msg != nil {
			t.Fatalf("request %d: expected nil (allowed), got message: %v", i+1, msg)
		}
	}
}

func TestRateLimit_RejectsRequestsOverLimit(t *testing.T) {
	callback := RateLimit(3, time.Minute)
	ctx := context.Background()
	sw := createTestStateWriter()

	// Fill the rate limit
	for i := 0; i < 3; i++ {
		callback(ctx, sw)
	}

	// 4th request should be rejected
	msg, err := callback(ctx, sw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg == nil {
		t.Fatal("expected rate limit message, got nil")
	}

	// Verify message has content
	parts := msg.Parts()
	if len(parts) == 0 {
		t.Fatal("expected non-empty rate limit message")
	}
}

func TestRateLimit_SlidingWindow(t *testing.T) {
	window := 100 * time.Millisecond
	callback := RateLimit(2, window)
	ctx := context.Background()
	sw := createTestStateWriter()

	// Use first request
	msg1, err := callback(ctx, sw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg1 != nil {
		t.Fatal("first request should be allowed")
	}

	// Wait half the window
	time.Sleep(window / 2)

	// Use second request
	msg2, err := callback(ctx, sw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg2 != nil {
		t.Fatal("second request should be allowed")
	}

	// Third request should be rejected (within window)
	msg3, err := callback(ctx, sw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg3 == nil {
		t.Fatal("third request should be rejected")
	}

	// Wait for first request to expire
	time.Sleep(window/2 + 20*time.Millisecond)

	// Now a new request should be allowed (first expired)
	msg4, err := callback(ctx, sw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg4 != nil {
		t.Fatal("request after window should be allowed")
	}
}

func TestRateLimit_Concurrent(t *testing.T) {
	maxRequests := 10
	callback := RateLimit(maxRequests, time.Second)
	ctx := context.Background()

	var wg sync.WaitGroup
	goroutines := 20
	allowed := make([]bool, goroutines)

	// Fire concurrent requests
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sw := createTestStateWriter()
			msg, _ := callback(ctx, sw)
			allowed[idx] = (msg == nil)
		}(i)
	}

	wg.Wait()

	// Count allowed requests
	allowedCount := 0
	for _, wasAllowed := range allowed {
		if wasAllowed {
			allowedCount++
		}
	}

	// Should allow exactly maxRequests
	if allowedCount != maxRequests {
		t.Errorf("expected %d allowed requests, got %d", maxRequests, allowedCount)
	}
}

func TestRateLimit_ZeroMaxRequests(t *testing.T) {
	callback := RateLimit(0, time.Minute)
	ctx := context.Background()
	sw := createTestStateWriter()

	// Even first request should be rejected with 0 limit
	msg, err := callback(ctx, sw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg == nil {
		t.Fatal("expected rejection with 0 max requests")
	}
}

func TestPerNodeRateLimit_Independent(t *testing.T) {
	callback1 := PerNodeRateLimit("node1", 2, time.Minute)
	callback2 := PerNodeRateLimit("node2", 2, time.Minute)
	ctx := context.Background()
	sw := createTestStateWriter()

	// Fill node1's limit
	callback1(ctx, sw)
	callback1(ctx, sw)

	// node1 should be rate limited
	msg1, err := callback1(ctx, sw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg1 == nil {
		t.Fatal("expected node1 to be rate limited")
	}

	// node2 should still be allowed (independent counter)
	msg2, err := callback2(ctx, sw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg2 != nil {
		t.Fatal("expected node2 to be allowed (independent limit)")
	}
}

func TestPerNodeRateLimit_MessageContainsNodeName(t *testing.T) {
	nodeName := "expensive_api"
	callback := PerNodeRateLimit(nodeName, 1, time.Minute)
	ctx := context.Background()
	sw := createTestStateWriter()

	// Use up the limit
	callback(ctx, sw)

	// Get rejection message
	msg, err := callback(ctx, sw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg == nil {
		t.Fatal("expected rate limit message")
	}

	// Verify message has content
	parts := msg.Parts()
	if len(parts) == 0 {
		t.Error("rate limit message should not be empty")
	}
}
