package chanutil_test

import (
	"context"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/internal/chanutil"
)

func TestDrainUntilClosed(t *testing.T) {
	t.Run("drains all values and exits when closed", func(t *testing.T) {
		ch := make(chan int, 10)

		// Send values
		for i := range 10 {
			ch <- i
		}
		close(ch)

		// Drain should consume all values without blocking
		done := make(chan struct{})
		go func() {
			chanutil.DrainUntilClosed(ch)
			close(done)
		}()

		select {
		case <-done:
			// Success
		case <-time.After(100 * time.Millisecond):
			t.Fatal("DrainUntilClosed blocked")
		}
	})

	t.Run("unblocks sender", func(t *testing.T) {
		ch := make(chan int)

		// Start sender that would block
		senderDone := make(chan struct{})
		go func() {
			ch <- 1
			ch <- 2
			close(ch)
			close(senderDone)
		}()

		// Let sender send first value
		<-ch

		// Drain remaining values
		drainDone := make(chan struct{})
		go func() {
			chanutil.DrainUntilClosed(ch)
			close(drainDone)
		}()

		select {
		case <-senderDone:
			// Success - sender completed
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Sender blocked")
		}

		select {
		case <-drainDone:
			// Success - drain completed
		case <-time.After(100 * time.Millisecond):
			t.Fatal("DrainUntilClosed blocked")
		}
	})
}

func TestDrainWithContext(t *testing.T) {
	t.Run("drains until channel closed", func(t *testing.T) {
		ctx := context.Background()
		ch := make(chan int, 5)

		for i := range 5 {
			ch <- i
		}
		close(ch)

		done := make(chan struct{})
		go func() {
			chanutil.DrainWithContext(ctx, ch)
			close(done)
		}()

		select {
		case <-done:
			// Success
		case <-time.After(100 * time.Millisecond):
			t.Fatal("DrainWithContext blocked")
		}
	})

	t.Run("stops on context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		ch := make(chan int, 10)

		// Fill channel but don't close it
		for i := range 10 {
			ch <- i
		}

		done := make(chan struct{})
		go func() {
			chanutil.DrainWithContext(ctx, ch)
			close(done)
		}()

		// Cancel context
		cancel()

		select {
		case <-done:
			// Success - stopped on cancellation
		case <-time.After(100 * time.Millisecond):
			t.Fatal("DrainWithContext didn't stop on context cancellation")
		}
	})

	t.Run("respects context deadline", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		ch := make(chan int) // Unbuffered, never closes

		done := make(chan struct{})
		go func() {
			chanutil.DrainWithContext(ctx, ch)
			close(done)
		}()

		select {
		case <-done:
			// Success - stopped on deadline
		case <-time.After(200 * time.Millisecond):
			t.Fatal("DrainWithContext didn't stop on context deadline")
		}
	})
}
