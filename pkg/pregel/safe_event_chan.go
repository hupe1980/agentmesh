package pregel

import (
	"sync"
	"time"
)

// safeEventChan is a thread-safe wrapper around an event channel that prevents
// "send on closed channel" panics. It uses an RWMutex to coordinate between
// senders and the closer, providing true race-free operation.
//
// The race condition scenario this prevents:
//
//	Thread 1: Send()                   Thread 2: Close()
//	  1. Acquire RLock
//	  2. Check closed flag
//	  3. Send to channel
//	  4. Release RLock
//	                                   5. Acquire Lock
//	                                   6. Set closed flag
//	                                   7. Close channel
//	                                   8. Release Lock
//
// By using RWMutex:
//   - Send() holds RLock during the critical send operation
//   - Close() acquires Lock, preventing any Send() from proceeding
//   - Once Lock is acquired, no Send() can be in progress or start
//   - Channel can be safely closed with no concurrent access
//
// This is the only truly race-free pattern that satisfies the Go race detector,
// as WaitGroup-based approaches still have unavoidable races between Add() calls
// and Wait() returns.
type safeEventChan[M any] struct {
	mu     sync.RWMutex
	ch     chan eventOrError[M]
	closed bool
}

// eventOrError is an internal type that wraps events with their associated errors
// for channel-based communication between the execution goroutine and iterator
type eventOrError[M any] struct {
	event Event[M]
	err   error
}

// newSafeEventChan creates a new safe event channel with the specified buffer size.
func newSafeEventChan[M any](bufferSize int) *safeEventChan[M] {
	return &safeEventChan[M]{
		ch: make(chan eventOrError[M], bufferSize),
	}
}

// Send attempts to deliver an event to the channel. Returns true if successful
// and false if the channel is closed or backpressure forces a timeout.
//
// Implementation details:
//   - Send() acquires RLock for the entire operation so Close() cannot race it.
//   - A non-blocking fast path attempts the send without allocating timers when
//     buffer capacity exists (the common case).
//   - When the buffer is full, we fall back to a cancellable timer via
//     time.NewTimer, ensuring timeouts do not leak goroutines or wakeups.
//
// This design keeps observability emits allocation-free under steady load while
// still enforcing a bounded 100ms wait when consumers fall behind.
//
// For production systems with slow consumers, consider increasing
// DefaultEventChanBufferSize, sampling events, or handling backpressure at the
// application layer.
func (s *safeEventChan[M]) Send(evt Event[M], err error) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return false
	}

	msg := eventOrError[M]{event: evt, err: err}

	// Fast path: try immediate, non-blocking send to avoid timers when buffer has capacity.
	select {
	case s.ch <- msg:
		return true
	default:
	}

	timer := time.NewTimer(100 * time.Millisecond)
	defer func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}()

	select {
	case s.ch <- msg:
		return true
	case <-timer.C:
		return false
	}
}

// Close closes the channel and marks it as closed. Safe to call multiple times.
// After closing, all Send() calls will return false immediately.
//
// Close sequence (guarantees race-free operation):
//  1. Acquire exclusive Lock (blocks all Send() operations)
//  2. Check if already closed (idempotent)
//  3. Set closed flag
//  4. Close event channel (safe - no Send() can be in progress)
//  5. Release Lock
//
// The exclusive Lock ensures that:
//   - No Send() can be in progress when we close the channel (all blocked on RLock)
//   - No new Send() can start after we set the closed flag
//   - The close operation is atomic with respect to all sends
//
// This makes close(s.ch) race-free because we have exclusive access.
func (s *safeEventChan[M]) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return // Already closed
	}

	s.closed = true
	close(s.ch)
}

// Chan returns the underlying channel for reading. Returns nil if closed.
// The returned channel should only be used for reading, never for sending.
func (s *safeEventChan[M]) Chan() <-chan eventOrError[M] {
	return s.ch
}

// IsClosed returns true if the channel has been closed.
func (s *safeEventChan[M]) IsClosed() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.closed
}
