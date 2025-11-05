package stream

import (
	"context"
	"sync"
)

// Config customizes stream behavior for specific item types.
type Config[T any] struct {
	// ExtractErr returns the error contained in an item, if any. Optional.
	ExtractErr func(T) error
	// IsFinal reports whether the provided item represents the final element.
	// Optional; when nil the stream only finishes when the channel closes.
	IsFinal func(T) bool
	// StopOnErr stops iteration after the first non-nil error extracted via
	// ExtractErr. When false, iteration continues until the channel closes or
	// IsFinal returns true.
	StopOnErr bool
}

// Stream provides scanner-style access to items produced on a channel.
type Stream[T any] struct {
	items  <-chan T
	cancel context.CancelFunc
	cfg    Config[T]

	mu      sync.Mutex
	current T
	err     error
	done    bool
}

// New constructs a stream for the supplied channel and cancellation function.
func New[T any](items <-chan T, cancel context.CancelFunc, cfg Config[T]) *Stream[T] {
	return &Stream[T]{items: items, cancel: cancel, cfg: cfg}
}

// Next advances the stream and reports whether another item is available.
func (s *Stream[T]) Next() bool {
	if s == nil {
		return false
	}

	s.mu.Lock()
	if s.done {
		s.mu.Unlock()
		return false
	}
	s.mu.Unlock()

	item, ok := <-s.items
	if !ok {
		s.mu.Lock()
		var zero T
		s.current = zero
		s.done = true
		s.mu.Unlock()
		return false
	}

	s.mu.Lock()
	s.current = item

	if s.cfg.ExtractErr != nil {
		if err := s.cfg.ExtractErr(item); err != nil {
			if s.err == nil {
				s.err = err
			}
			if s.cfg.StopOnErr {
				s.done = true
			}
		}
	}

	if !s.done && s.cfg.IsFinal != nil && s.cfg.IsFinal(item) {
		s.done = true
	}
	s.mu.Unlock()

	return true
}

// Current returns the most recently observed item.
func (s *Stream[T]) Current() T {
	if s == nil {
		var zero T
		return zero
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current
}

// Err reports the first error observed while iterating.
func (s *Stream[T]) Err() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// Cancel terminates the stream early.
func (s *Stream[T]) Cancel() {
	if s == nil {
		return
	}

	s.mu.Lock()
	if s.done {
		s.mu.Unlock()
		return
	}
	s.done = true
	s.mu.Unlock()

	if s.cancel != nil {
		s.cancel()
	}
}
