package pregel

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestSafeEventChan_ConcurrentSendAndClose tests that Send and Close can be
// called concurrently without panicking or deadlocking.
func TestSafeEventChan_ConcurrentSendAndClose(t *testing.T) {
	const (
		numSenders     = 100
		sendsPerWorker = 1000
	)

	ch := newSafeEventChan[any](10)

	var wg sync.WaitGroup
	wg.Add(numSenders)

	// Start many concurrent senders
	for i := 0; i < numSenders; i++ {
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < sendsPerWorker; j++ {
				ch.Send(Event[any]{Node: "test"})
			}
		}(i)
	}

	// Close the channel while senders are still active
	time.Sleep(10 * time.Millisecond)
	ch.Close()

	// Wait for all senders to finish (should not panic)
	wg.Wait()

	// Verify channel is closed
	if !ch.IsClosed() {
		t.Error("Channel should be closed")
	}

	// Additional sends should return false
	if ch.Send(Event[any]{Node: "after-close"}) {
		t.Error("Send should return false after close")
	}
}

// TestSafeEventChan_MultipleClose tests that closing multiple times is safe.
func TestSafeEventChan_MultipleClose(t *testing.T) {
	ch := newSafeEventChan[any](10)

	// Close multiple times - should not panic
	ch.Close()
	ch.Close()
	ch.Close()

	if !ch.IsClosed() {
		t.Error("Channel should be closed")
	}
}

// TestSafeEventChan_SendTimeout tests that Send times out on a full channel.
func TestSafeEventChan_SendTimeout(t *testing.T) {
	ch := newSafeEventChan[any](1)

	// Fill the buffer
	if !ch.Send(Event[any]{Node: "first"}) {
		t.Fatal("First send should succeed")
	}

	// This should timeout since no one is receiving
	start := time.Now()
	result := ch.Send(Event[any]{Node: "second"})
	elapsed := time.Since(start)

	if result {
		t.Error("Send should return false when channel is full and times out")
	}

	// Verify timeout actually occurred (should be around 100ms)
	if elapsed < 90*time.Millisecond || elapsed > 200*time.Millisecond {
		t.Errorf("Timeout took %v, expected ~100ms", elapsed)
	}
}

// TestSafeEventChan_NormalUsage tests normal send/receive flow.
func TestSafeEventChan_NormalUsage(t *testing.T) {
	ch := newSafeEventChan[any](10)

	// Send some events
	for i := 0; i < 5; i++ {
		if !ch.Send(Event[any]{Superstep: int64(i)}) {
			t.Fatalf("Send %d failed", i)
		}
	}

	// Receive them
	var received int
	var receivedMu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		for evt := range ch.Chan() {
			receivedMu.Lock()
			if evt.Superstep != int64(received) {
				t.Errorf("Expected superstep %d, got %d", received, evt.Superstep)
			}
			received++
			receivedMu.Unlock()
		}
	}()

	// Close and wait
	time.Sleep(10 * time.Millisecond)
	ch.Close()
	wg.Wait()

	receivedMu.Lock()
	defer receivedMu.Unlock()
	if received != 5 {
		t.Errorf("Expected to receive 5 events, got %d", received)
	}
}

// TestRuntime_RaceConditionFix_ConcurrentEmit tests that emitEvent can be
// called concurrently while Run is closing without panicking.
// This test specifically targets the race condition described in FINDINGS.md.
func TestRuntime_RaceConditionFix_ConcurrentEmit(t *testing.T) {
	const numWorkers = 50

	var callCount int
	var sent []Message[mockMessage]
	mu1, mu2 := &sync.Mutex{}, &sync.Mutex{}

	// Create a graph that runs a few iterations
	g := &mockGraph{
		rootNodes: []string{"root"},
		nodes: map[string]*mockNode{
			"root": {
				name:       "root",
				next:       "root", // Send to self
				called:     &callCount,
				callMu:     mu1,
				messages:   &sent,
				messagesMu: mu2,
			},
		},
	}

	rt, err := NewRuntime(g, WithMaxIterations[mockState, mockMessage](10))
	if err != nil {
		t.Fatalf("Failed to create runtime: %v", err)
	}

	var wg sync.WaitGroup
	ctx := context.Background()

	// Start many goroutines that will call emitEvent concurrently
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			// Continuously emit events
			for j := 0; j < 100; j++ {
				rt.emitEvent(Event[mockMessage]{Node: "test-worker", Superstep: int64(j)})
				time.Sleep(time.Microsecond)
			}
		}(i)
	}

	// Run the runtime (will close eventChan when done)
	go func() {
		for range rt.Run(ctx) {
			// Consume events
		}
	}()

	// Let it run briefly then wait for workers
	time.Sleep(50 * time.Millisecond)
	wg.Wait()

	// If we got here without panicking, the fix worked!
	t.Log("✓ No panic occurred during concurrent emit and close")
}

// TestRuntime_EmitEventAfterClose tests that emitEvent returns false after
// the runtime has been closed, and doesn't panic.
func TestRuntime_EmitEventAfterClose(t *testing.T) {
	var callCount int
	var sent []Message[mockMessage]
	mu1, mu2 := &sync.Mutex{}, &sync.Mutex{}

	g := &mockGraph{
		rootNodes: []string{"root"},
		nodes: map[string]*mockNode{
			"root": {
				name:       "root",
				called:     &callCount,
				callMu:     mu1,
				messages:   &sent,
				messagesMu: mu2,
				// No next node - will quiesce immediately
			},
		},
	}

	rt, err := NewRuntime(g)
	if err != nil {
		t.Fatalf("Failed to create runtime: %v", err)
	}

	ctx := context.Background()

	// Run to completion
	for range rt.Run(ctx) {
		// Consume events
	}

	// Try to emit after run completed - should return false, not panic
	result := rt.emitEvent(Event[mockMessage]{Node: "after-close"})
	if result {
		t.Error("emitEvent should return false after runtime closed")
	}

	// Multiple calls should also be safe
	for i := 0; i < 10; i++ {
		if rt.emitEvent(Event[mockMessage]{Node: "test"}) {
			t.Error("emitEvent should return false after runtime closed")
		}
	}

	t.Log("✓ emitEvent handled gracefully after close")
}
