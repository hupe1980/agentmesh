package pregel

import (
	"sync"
	"time"
)

// safeEventChan is a thread-safe wrapper around an event channel that prevents
// "send on closed channel" panics. It handles the race condition where one
// goroutine attempts to send while another closes the channel.
//
// The race condition scenario this prevents:
//
//	Thread 1: emitEvent()              Thread 2: Run() cleanup
//	  1. Acquire RLock
//	  2. Read channel reference
//	  3. Release RLock
//	                                   4. Acquire Lock
//	                                   5. Close channel
//	                                   6. Set channel to nil
//	                                   7. Release Lock
//	  8. Send to channel -> PANIC!
//
// This wrapper ensures atomic check-and-send operations, preventing panics
// even under high concurrency with multiple workers emitting events while
// the channel is being closed.
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

// Send attempts to send an event to the channel. Returns true if successful,
// false if the channel is closed or the send times out.
//
// The method uses a timeout to prevent indefinite blocking if the consumer
// is slow or has stopped reading. A 100ms timeout is chosen as a reasonable
// balance between:
//   - Allowing burst handling (consumers typically read in tight loops)
//   - Preventing indefinite blocking on a full channel
//   - Avoiding excessive goroutine buildup
//
// For production systems with slow consumers, consider:
//   - Increasing buffer size (DefaultEventChanBufferSize)
//   - Using backpressure at the application level
//   - Implementing event sampling/aggregation
func (s *safeEventChan[M]) Send(evt Event[M], err error) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed || s.ch == nil {
		return false
	}

	// Use select with timeout to prevent indefinite blocking
	// This handles the case where the consumer is slow or has stopped reading
	select {
	case s.ch <- eventOrError[M]{event: evt, err: err}:
		return true
	case <-time.After(100 * time.Millisecond):
		// Timeout occurred - channel is likely full or consumer is slow
		// This is not an error condition, just backpressure
		return false
	}
}

// Close closes the channel and marks it as closed. Safe to call multiple times.
// After closing, all Send() calls will return false immediately.
func (s *safeEventChan[M]) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.closed && s.ch != nil {
		close(s.ch)
		s.closed = true
		s.ch = nil
	}
}

// Chan returns the underlying channel for reading. Returns nil if closed.
// The returned channel should only be used for reading, never for sending.
func (s *safeEventChan[M]) Chan() <-chan eventOrError[M] {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ch
}

// IsClosed returns true if the channel has been closed.
func (s *safeEventChan[M]) IsClosed() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.closed
}
