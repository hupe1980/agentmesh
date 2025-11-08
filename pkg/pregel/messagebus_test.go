package pregel

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestInMemoryMessageBus_Basic(t *testing.T) {
	bus := NewInMemoryMessageBus[string](0, nil)
	defer bus.Close()

	// Test sending messages
	messages := []Message[string]{
		{From: "a", To: "b", Data: "msg1"},
		{From: "a", To: "c", Data: "msg2"},
	}

	err := bus.Send(t.Context(), messages)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Test receiving messages
	msgsB, err := bus.Receive("b")
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}
	if len(msgsB) != 1 || msgsB[0].Data != "msg1" {
		t.Errorf("Expected 1 message for b, got %d", len(msgsB))
	}

	msgsC, err := bus.Receive("c")
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}
	if len(msgsC) != 1 || msgsC[0].Data != "msg2" {
		t.Errorf("Expected 1 message for c, got %d", len(msgsC))
	}

	// Mailbox should be empty after receive
	msgsB2, err := bus.Receive("b")
	if err != nil {
		t.Fatalf("Second receive failed: %v", err)
	}
	if len(msgsB2) != 0 {
		t.Errorf("Expected empty mailbox after receive, got %d messages", len(msgsB2))
	}
}

func TestInMemoryMessageBus_MaxSize(t *testing.T) {
	bus := NewInMemoryMessageBus[string](2, nil)
	defer bus.Close()

	ctx := t.Context()

	// Send 2 messages - should succeed (fills the channel buffer)
	err := bus.Send(ctx, []Message[string]{
		{To: "a", Data: "msg1"},
		{To: "a", Data: "msg2"},
	})
	if err != nil {
		t.Fatalf("Send of 2 messages failed: %v", err)
	}

	// With backpressure, send 3rd message in goroutine - it should block since mailbox is full
	done := make(chan error, 1)
	go func() {
		done <- bus.Send(ctx, []Message[string]{
			{To: "a", Data: "msg3"},
		})
	}()

	// Verify send is blocked (give it a moment)
	select {
	case <-done:
		t.Error("Expected send to block when mailbox full")
	case <-time.After(50 * time.Millisecond):
		// Good - send is blocked
	}

	// Now receive messages - this drains the channel creating space
	msgs, _ := bus.Receive("a")
	// After draining, the blocked send completes, so we get all 3 messages
	if len(msgs) < 2 {
		t.Errorf("Expected at least 2 messages, got %d", len(msgs))
	}

	// The blocked send should now complete
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Send should succeed after receive, got error: %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Send should have unblocked after receive")
	}

	// If msg3 wasn't in the first receive, it should be available now
	if len(msgs) == 2 {
		msgs, _ = bus.Receive("a")
		if len(msgs) != 1 || msgs[0].Data != "msg3" {
			t.Errorf("Expected msg3 to be delivered after unblocking, got %d messages", len(msgs))
		}
	} else if len(msgs) == 3 {
		// All messages received in one go - verify msg3 is present
		found := false
		for _, m := range msgs {
			if m.Data == "msg3" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected msg3 to be in the received messages")
		}
	}
}

func TestInMemoryMessageBus_Combiner(t *testing.T) {
	// Combiner that concatenates string data
	combiner := func(existing, incoming Message[string]) Message[string] {
		return Message[string]{
			From: existing.From,
			To:   existing.To,
			Data: existing.Data + "," + incoming.Data,
		}
	}

	bus := NewInMemoryMessageBus[string](0, combiner)
	defer bus.Close()

	// Send multiple messages to same target
	err := bus.Send(context.Background(), []Message[string]{
		{To: "a", Data: "msg1"},
		{To: "a", Data: "msg2"},
		{To: "a", Data: "msg3"},
	})
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Should have only 1 combined message
	msgs, err := bus.Receive("a")
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Errorf("Expected 1 combined message, got %d", len(msgs))
	}
	if msgs[0].Data != "msg1,msg2,msg3" {
		t.Errorf("Expected combined data 'msg1,msg2,msg3', got %q", msgs[0].Data)
	}
}

func TestInMemoryMessageBus_Pending(t *testing.T) {
	bus := NewInMemoryMessageBus[string](0, nil)
	defer bus.Close()

	// Send messages to multiple targets
	err := bus.Send(context.Background(), []Message[string]{
		{To: "a", Data: "msg1"},
		{To: "b", Data: "msg2"},
		{To: "c", Data: "msg3"},
	})
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Get pending vertices
	pending, err := bus.Pending()
	if err != nil {
		t.Fatalf("Pending failed: %v", err)
	}
	if len(pending) != 3 {
		t.Errorf("Expected 3 pending vertices, got %d", len(pending))
	}

	// Second call should return empty (frontier consumed)
	pending2, err := bus.Pending()
	if err != nil {
		t.Fatalf("Second Pending failed: %v", err)
	}
	if len(pending2) != 0 {
		t.Errorf("Expected empty frontier after consumption, got %d", len(pending2))
	}
}

func TestInMemoryMessageBus_Clear(t *testing.T) {
	bus := NewInMemoryMessageBus[string](0, nil)
	defer bus.Close()

	// Send messages
	err := bus.Send(context.Background(), []Message[string]{
		{To: "a", Data: "msg1"},
	})
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Clear mailbox
	err = bus.Clear("a")
	if err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	// Mailbox should be empty
	msgs, err := bus.Receive("a")
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("Expected empty mailbox after clear, got %d messages", len(msgs))
	}
}

func TestInMemoryMessageBus_Concurrent(t *testing.T) {
	bus := NewInMemoryMessageBus[int](0, nil)
	defer bus.Close()

	var wg sync.WaitGroup
	numGoroutines := 10
	messagesPerGoroutine := 100

	// Concurrent senders
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < messagesPerGoroutine; j++ {
				msg := Message[int]{
					From: "sender",
					To:   "target",
					Data: id*1000 + j,
				}
				_ = bus.Send(context.Background(), []Message[int]{msg})
			}
		}(i)
	}

	wg.Wait()

	// Receive all messages
	msgs, err := bus.Receive("target")
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}

	expectedCount := numGoroutines * messagesPerGoroutine
	if len(msgs) != expectedCount {
		t.Errorf("Expected %d messages, got %d", expectedCount, len(msgs))
	}
}

func TestInMemoryMessageBus_Stats(t *testing.T) {
	bus := NewInMemoryMessageBus[string](0, nil)
	defer bus.Close()

	// Send messages to multiple targets
	err := bus.Send(context.Background(), []Message[string]{
		{To: "a", Data: "msg1"},
		{To: "a", Data: "msg2"},
		{To: "b", Data: "msg3"},
	})
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	stats := bus.Stats()
	if stats.TotalMessages != 3 {
		t.Errorf("Expected 3 total messages, got %d", stats.TotalMessages)
	}
	if stats.VerticesWithMessages != 2 {
		t.Errorf("Expected 2 vertices with messages, got %d", stats.VerticesWithMessages)
	}
	if stats.LargestMailbox != 2 {
		t.Errorf("Expected largest mailbox 2, got %d", stats.LargestMailbox)
	}
}

func TestInMemoryMessageBus_EmptyTarget(t *testing.T) {
	bus := NewInMemoryMessageBus[string](0, nil)
	defer bus.Close()

	// Send message with empty target - should be ignored
	err := bus.Send(context.Background(), []Message[string]{
		{To: "", Data: "msg1"},
		{To: "a", Data: "msg2"},
	})
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Only "a" should have messages
	pending, _ := bus.Pending()
	if len(pending) != 1 || pending[0] != "a" {
		t.Errorf("Expected only 'a' in pending, got %v", pending)
	}
}
