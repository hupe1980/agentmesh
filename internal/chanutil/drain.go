// Package chanutil provides utility functions for working with Go channels.
package chanutil

import "context"

// DrainUntilClosed drains all values from a channel until it's closed.
// This is useful for unblocking senders when a receiver stops consuming early.
//
// Use case: Prevent goroutine leaks when a producer is still sending but
// the consumer has stopped. The empty loop body is intentional - we're
// discarding values to unblock the sender.
//
// Example:
//
//	// Producer goroutine
//	go func() {
//	    for i := 0; i < 100; i++ {
//	        resultChan <- i
//	    }
//	    close(resultChan)
//	}()
//
//	// Consumer stops early
//	for result := range resultChan {
//	    if shouldStop(result) {
//	        chanutil.DrainUntilClosed(resultChan)
//	        break
//	    }
//	}
func DrainUntilClosed[T any](ch <-chan T) {
	//nolint:revive // Empty loop body is intentional - draining channel to unblock sender
	for range ch {
		// Discard all remaining values to unblock sender
	}
}

// DrainWithContext drains a channel until it's closed or the context is cancelled.
// This ensures responsive shutdown when context cancellation is the priority.
//
// Use case: When you need to stop draining immediately on context cancellation
// rather than waiting for the channel to close naturally.
//
// Example:
//
//	select {
//	case <-ctx.Done():
//	    chanutil.DrainWithContext(ctx, resultChan)
//	    return
//	case result := <-resultChan:
//	    process(result)
//	}
func DrainWithContext[T any](ctx context.Context, ch <-chan T) {
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return // Channel closed
			}
			// Discard value, continue draining
		case <-ctx.Done():
			return // Context cancelled, stop draining
		}
	}
}
