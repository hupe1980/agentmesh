package pregel

import (
	"context"
	"sync"
	"sync/atomic"
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
	for i := range numSenders {
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < sendsPerWorker; j++ {
				ch.Send(Event[any]{Vertex: "test"}, nil)
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
	if ch.Send(Event[any]{Vertex: "after-close"}, nil) {
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
	if !ch.Send(Event[any]{Vertex: "first"}, nil) {
		t.Fatal("First send should succeed")
	}

	// This should timeout since no one is receiving
	start := time.Now()
	result := ch.Send(Event[any]{Vertex: "second"}, nil)
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
	for i := range 5 {
		if !ch.Send(Event[any]{Superstep: int64(i)}, nil) {
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
		for eoe := range ch.Chan() {
			receivedMu.Lock()
			if eoe.event.Superstep != int64(received) {
				t.Errorf("Expected superstep %d, got %d", received, eoe.event.Superstep)
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
	for i := range numWorkers {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			// Continuously emit events
			for j := 0; j < 100; j++ {
				rt.emitEvent(Event[mockMessage]{Vertex: "test-worker", Superstep: int64(j)}, nil)
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
	result := rt.emitEvent(Event[mockMessage]{Vertex: "after-close"}, nil)
	if result {
		t.Error("emitEvent should return false after runtime closed")
	}

	// Multiple calls should also be safe
	for i := 0; i < 10; i++ {
		if rt.emitEvent(Event[mockMessage]{Vertex: "test"}, nil) {
			t.Error("emitEvent should return false after runtime closed")
		}
	}

	t.Log("✓ emitEvent handled gracefully after close")
}

// TestSafeEventChan_FastPathAllocs ensures the fast-path send stays allocation-free.
func TestSafeEventChan_FastPathAllocs(t *testing.T) {
	ch := newSafeEventChan[int](1024)
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-ch.Chan():
			}
		}
	}()

	t.Cleanup(func() {
		close(stop)
		ch.Close()
	})

	allocs := testing.AllocsPerRun(1000, func() {
		if !ch.Send(Event[int]{Superstep: 1}, nil) {
			t.Fatal("expected send to succeed")
		}
	})

	if allocs > 0.5 {
		t.Fatalf("expected ~0 allocations per send, got %.2f", allocs)
	}
}

// TestSafeEventChan_BackpressureUnderLoad verifies we still enforce timeouts
// when the buffer stays full under contention.
func TestSafeEventChan_BackpressureUnderLoad(t *testing.T) {
	ch := newSafeEventChan[int](1)
	if !ch.Send(Event[int]{Vertex: "seed"}, nil) {
		t.Fatal("failed to seed buffer")
	}

	const workers = 4
	var timedOut atomic.Int32
	var wg sync.WaitGroup
	start := time.Now()

	for i := range workers {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			if ch.Send(Event[int]{Vertex: "blocked"}, nil) {
				t.Errorf("worker %d unexpectedly succeeded despite full buffer", id)
				return
			}
			timedOut.Add(1)
		}(i)
	}

	wg.Wait()
	elapsed := time.Since(start)

	if got := timedOut.Load(); got != workers {
		t.Fatalf("expected %d timeouts, got %d", workers, got)
	}

	if elapsed < 90*time.Millisecond || elapsed > 250*time.Millisecond {
		t.Fatalf("expected ~100ms backpressure window, got %v", elapsed)
	}

	ch.Close()
}

func BenchmarkSafeEventChanSendBuffered(b *testing.B) {
	ch := newSafeEventChan[int](2048)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			case <-ch.Chan():
			}
		}
	}()

	defer func() {
		close(done)
		ch.Close()
	}()

	b.ReportAllocs()

	for b.Loop() {
		if !ch.Send(Event[int]{Vertex: "bench"}, nil) {
			b.Fatal("send failed in benchmark")
		}
	}
}
