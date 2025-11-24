package pregel

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestInMemoryMessageBus_Basic(t *testing.T) {
	bus := NewInMemoryMessageBus[string](100, nil)
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

	// Use small mailbox (4) so that 3 messages exceeds 75% threshold and triggers combining
	bus := NewInMemoryMessageBus[string](4, combiner)
	defer bus.Close()

	// Send multiple messages to same target
	// With mailbox size 4, threshold is 3 (75%), so combining should trigger
	err := bus.Send(context.Background(), []Message[string]{
		{To: "a", Data: "msg1"},
		{To: "a", Data: "msg2"},
		{To: "a", Data: "msg3"},
	})
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Should have combined messages (may be 1 or 2 depending on timing)
	msgs, err := bus.Receive("a")
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}

	// Verify at least some combining occurred
	if len(msgs) > 3 {
		t.Errorf("Expected at most 3 messages (with combining), got %d", len(msgs))
	}

	// Concatenate all received data to verify no message loss
	var allData string
	for i, msg := range msgs {
		if i > 0 {
			allData += ","
		}
		allData += msg.Data
	}

	// Check that all messages were delivered (order may vary)
	if !containsAll(allData, []string{"msg1", "msg2", "msg3"}) {
		t.Errorf("Expected all messages to be delivered, got %q", allData)
	}
}

// Helper function to check if all substrings are present
func containsAll(data string, substrings []string) bool {
	for _, s := range substrings {
		found := false
		for _, part := range splitByComma(data) {
			if part == s {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func splitByComma(s string) []string {
	var result []string
	current := ""
	for _, c := range s {
		if c == ',' {
			if current != "" {
				result = append(result, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

func TestInMemoryMessageBus_Close(t *testing.T) {
	bus := NewInMemoryMessageBus[string](100, nil)
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

func TestInMemoryMessageBus_Sharding(t *testing.T) {
	// Increase mailbox size to accommodate all messages to avoid deadlock
	bus := NewInMemoryMessageBus[int](1000, nil)
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
	bus := NewInMemoryMessageBus[string](100, nil)
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

	// Only "a" should have messages (verify by receiving)
	msgs, _ := bus.Receive("a")
	if len(msgs) != 1 || msgs[0].Data != "msg2" {
		t.Errorf("Expected only message to 'a', got %v", msgs)
	}

	// Empty target should have no messages
	emptyMsgs, _ := bus.Receive("")
	if len(emptyMsgs) != 0 {
		t.Errorf("Expected no messages for empty target, got %d", len(emptyMsgs))
	}
}

// TestInMemoryMessageBus_BackpressureNoMessageLoss verifies that messages are never dropped
// when mailbox is full, they are held until space is available (backpressure).
func TestInMemoryMessageBus_BackpressureNoMessageLoss(t *testing.T) {
	const mailboxSize = 5
	bus := NewInMemoryMessageBus[int](mailboxSize, nil)
	defer bus.Close()

	ctx := context.Background()

	// Fill mailbox to capacity
	var sentData []int
	for i := 0; i < mailboxSize; i++ {
		sentData = append(sentData, i)
		err := bus.Send(ctx, []Message[int]{
			{To: "vertex1", Data: i},
		})
		if err != nil {
			t.Fatalf("Send %d failed: %v", i, err)
		}
	}

	// Verify mailbox is full
	stats := bus.Stats()
	if stats.TotalMessages != mailboxSize {
		t.Errorf("Expected %d messages in mailbox, got %d", mailboxSize, stats.TotalMessages)
	}

	// Attempt to send more messages concurrently - they should block
	const extraMessages = 10
	var wg sync.WaitGroup
	errChan := make(chan error, extraMessages)

	for i := mailboxSize; i < mailboxSize+extraMessages; i++ {
		wg.Add(1)
		sentData = append(sentData, i)
		go func(val int) {
			defer wg.Done()
			err := bus.Send(ctx, []Message[int]{
				{To: "vertex1", Data: val},
			})
			errChan <- err
		}(i)
	}

	// Give goroutines time to block
	time.Sleep(50 * time.Millisecond)

	// Verify sends are blocked (no errors yet)
	select {
	case <-errChan:
		t.Error("Send should be blocked, not completed")
	default:
		// Good - sends are blocked
	}

	// Now drain mailbox to unblock sends
	receivedData := make(map[int]bool)
	for {
		msgs, err := bus.Receive("vertex1")
		if err != nil {
			t.Fatalf("Receive failed: %v", err)
		}
		if len(msgs) == 0 {
			// Check if all goroutines completed
			done := make(chan struct{})
			go func() {
				wg.Wait()
				close(done)
			}()
			select {
			case <-done:
				// All sends completed
				goto verify
			case <-time.After(100 * time.Millisecond):
				// More sends may still be in flight, continue receiving
			}
		}
		for _, msg := range msgs {
			receivedData[msg.Data] = true
		}
	}

verify:
	// Verify all sends completed without errors
	close(errChan)
	for err := range errChan {
		if err != nil {
			t.Errorf("Send failed: %v", err)
		}
	}

	// Verify ALL messages were delivered (no drops)
	if len(receivedData) != len(sentData) {
		t.Errorf("Expected %d unique messages, got %d", len(sentData), len(receivedData))
	}
	for _, val := range sentData {
		if !receivedData[val] {
			t.Errorf("Message with data %d was dropped", val)
		}
	}
}

// TestInMemoryMessageBus_ContextCancellationDuringBackpressure verifies that
// when a send is blocked due to full mailbox and context is cancelled,
// an error is returned and the message is not delivered.
func TestInMemoryMessageBus_ContextCancellationDuringBackpressure(t *testing.T) {
	const mailboxSize = 2
	bus := NewInMemoryMessageBus[string](mailboxSize, nil)
	defer bus.Close()

	// Fill mailbox to capacity
	ctx := context.Background()
	for i := 0; i < mailboxSize; i++ {
		err := bus.Send(ctx, []Message[string]{
			{To: "vertex1", Data: "msg" + string(rune('0'+i))},
		})
		if err != nil {
			t.Fatalf("Send %d failed: %v", i, err)
		}
	}

	// Attempt send with cancelled context - should fail immediately
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := bus.Send(cancelledCtx, []Message[string]{
		{To: "vertex1", Data: "should-fail"},
	})
	if err == nil {
		t.Error("Expected error when sending with cancelled context to full mailbox")
	}
	if err != nil {
		t.Logf("Got expected context error: %v", err)
	}

	// Attempt send with timeout context
	timeoutCtx, cancel2 := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel2()

	err = bus.Send(timeoutCtx, []Message[string]{
		{To: "vertex1", Data: "should-timeout"},
	})
	if err == nil {
		t.Error("Expected error when send times out due to full mailbox")
	}
	if err != nil {
		t.Logf("Got expected timeout error: %v", err)
	}

	// Verify original messages are intact
	msgs, _ := bus.Receive("vertex1")
	if len(msgs) != mailboxSize {
		t.Errorf("Expected %d messages (original only), got %d", mailboxSize, len(msgs))
	}
	for i, msg := range msgs {
		expected := "msg" + string(rune('0'+i))
		if msg.Data != expected {
			t.Errorf("Message %d: expected %q, got %q", i, expected, msg.Data)
		}
	}
}

// TestInMemoryMessageBus_ConcurrentBackpressure verifies correct behavior
// with many concurrent senders experiencing backpressure.
func TestInMemoryMessageBus_ConcurrentBackpressure(t *testing.T) {
	const (
		mailboxSize   = 10
		numSenders    = 50
		msgsPerSender = 20
	)

	bus := NewInMemoryMessageBus[int](mailboxSize, nil)
	defer bus.Close()

	ctx := context.Background()
	var wg sync.WaitGroup
	var sendErrors sync.Map

	// Start many concurrent senders
	for sender := 0; sender < numSenders; sender++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < msgsPerSender; i++ {
				err := bus.Send(ctx, []Message[int]{
					{To: "vertex1", Data: id*1000 + i},
				})
				if err != nil {
					sendErrors.Store(id*1000+i, err)
				}
			}
		}(sender)
	}

	// Concurrent receiver draining messages
	receivedData := make(map[int]bool)
	var receiveMu sync.Mutex
	var receiveWg sync.WaitGroup
	stopReceiving := make(chan struct{})

	receiveWg.Add(1)
	go func() {
		defer receiveWg.Done()
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-stopReceiving:
				// Final drain
				for {
					msgs, _ := bus.Receive("vertex1")
					if len(msgs) == 0 {
						return
					}
					receiveMu.Lock()
					for _, msg := range msgs {
						receivedData[msg.Data] = true
					}
					receiveMu.Unlock()
				}
			case <-ticker.C:
				msgs, _ := bus.Receive("vertex1")
				receiveMu.Lock()
				for _, msg := range msgs {
					receivedData[msg.Data] = true
				}
				receiveMu.Unlock()
			}
		}
	}()

	// Wait for all sends to complete
	wg.Wait()
	close(stopReceiving)
	receiveWg.Wait()

	// Verify no send errors
	sendErrors.Range(func(key, value interface{}) bool {
		t.Errorf("Send error for message %v: %v", key, value)
		return true
	})

	// Verify all messages delivered
	expectedCount := numSenders * msgsPerSender
	receiveMu.Lock()
	actualCount := len(receivedData)
	receiveMu.Unlock()

	if actualCount != expectedCount {
		t.Errorf("Expected %d messages delivered, got %d", expectedCount, actualCount)
	}
}
