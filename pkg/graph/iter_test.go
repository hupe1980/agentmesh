package graph_test

import (
	"errors"
	"iter"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/graph"
)

func TestLast(t *testing.T) {
	t.Run("returns last value", func(t *testing.T) {
		seq := func(yield func(int, error) bool) {
			yield(1, nil)
			yield(2, nil)
			yield(3, nil)
		}

		last, err := graph.Last(iter.Seq2[int, error](seq))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if last != 3 {
			t.Errorf("expected 3, got %d", last)
		}
	})

	t.Run("returns error on empty sequence", func(t *testing.T) {
		seq := func(yield func(int, error) bool) {
			// Empty sequence
		}

		_, err := graph.Last(iter.Seq2[int, error](seq))
		if !errors.Is(err, graph.ErrEmptySequence) {
			t.Errorf("expected ErrEmptySequence, got %v", err)
		}
	})

	t.Run("returns error from sequence", func(t *testing.T) {
		testErr := errors.New("test error")
		seq := func(yield func(int, error) bool) {
			yield(1, nil)
			yield(0, testErr)
			yield(3, nil)
		}

		_, err := graph.Last(iter.Seq2[int, error](seq))
		if !errors.Is(err, testErr) {
			t.Errorf("expected test error, got %v", err)
		}
	})

	t.Run("single value", func(t *testing.T) {
		seq := func(yield func(string, error) bool) {
			yield("only", nil)
		}

		last, err := graph.Last(iter.Seq2[string, error](seq))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if last != "only" {
			t.Errorf("expected 'only', got %q", last)
		}
	})
}

func TestCollect(t *testing.T) {
	t.Run("collects all values", func(t *testing.T) {
		seq := func(yield func(int, error) bool) {
			yield(1, nil)
			yield(2, nil)
			yield(3, nil)
		}

		results, err := graph.Collect(iter.Seq2[int, error](seq))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 3 {
			t.Fatalf("expected 3 results, got %d", len(results))
		}
		for i, v := range results {
			if v != i+1 {
				t.Errorf("results[%d] = %d, want %d", i, v, i+1)
			}
		}
	})

	t.Run("returns empty slice for empty sequence", func(t *testing.T) {
		seq := func(yield func(int, error) bool) {
			// Empty sequence
		}

		results, err := graph.Collect(iter.Seq2[int, error](seq))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("expected empty slice, got %v", results)
		}
	})

	t.Run("returns partial results on error", func(t *testing.T) {
		testErr := errors.New("test error")
		seq := func(yield func(int, error) bool) {
			yield(1, nil)
			yield(2, nil)
			yield(0, testErr)
		}

		results, err := graph.Collect(iter.Seq2[int, error](seq))
		if !errors.Is(err, testErr) {
			t.Errorf("expected test error, got %v", err)
		}
		if len(results) != 2 {
			t.Errorf("expected 2 partial results, got %d", len(results))
		}
	})

	t.Run("handles structs", func(t *testing.T) {
		type item struct {
			name string
		}
		seq := func(yield func(item, error) bool) {
			yield(item{"a"}, nil)
			yield(item{"b"}, nil)
		}

		results, err := graph.Collect(iter.Seq2[item, error](seq))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 2 {
			t.Fatalf("expected 2 results, got %d", len(results))
		}
		if results[0].name != "a" || results[1].name != "b" {
			t.Errorf("unexpected results: %v", results)
		}
	})
}
