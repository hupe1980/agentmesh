package retrieval

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type stubRetriever struct {
	docs []Document
	err  error
	hook func(context.Context)
}

func (s *stubRetriever) Retrieve(ctx context.Context, query string) ([]Document, error) {
	if s.hook != nil {
		s.hook(ctx)
	}
	if s.err != nil {
		return nil, s.err
	}
	return s.docs, nil
}

func TestMergerRetriever_MergeDocuments(t *testing.T) {
	retrievers := []Retriever{
		&stubRetriever{docs: []Document{{PageContent: "doc1", Metadata: map[string]any{"source": "a"}}}},
		&stubRetriever{docs: []Document{{PageContent: "doc2", Metadata: map[string]any{"source": "b"}}}},
	}

	retriever := NewMergerRetriever(retrievers, func(o *MergerRetrieverOptions) {
		o.StopOnFirstError = false
	})

	docs, err := retriever.Retrieve(context.Background(), "query")
	require.NoError(t, err)
	require.Len(t, docs, 2)
	require.Equal(t, "doc1", docs[0].PageContent)
	require.Equal(t, "doc2", docs[1].PageContent)
}

func TestMergerRetriever_ErrorAggregation(t *testing.T) {
	boom := errors.New("boom")
	retrievers := []Retriever{
		&stubRetriever{err: boom},
		&stubRetriever{docs: []Document{{PageContent: "doc"}}},
	}

	retriever := NewMergerRetriever(retrievers, func(o *MergerRetrieverOptions) {
		o.StopOnFirstError = false
	})

	docs, err := retriever.Retrieve(context.Background(), "query")
	require.ErrorIs(t, err, boom)
	require.Len(t, docs, 1)
	require.Equal(t, "doc", docs[0].PageContent)
}

func TestMergerRetriever_SkipNilRetrievers(t *testing.T) {
	retrievers := []Retriever{
		nil,
		&stubRetriever{docs: []Document{{PageContent: "doc"}}},
	}

	retriever := NewMergerRetriever(retrievers, func(o *MergerRetrieverOptions) {
		o.StopOnFirstError = false
	})

	docs, err := retriever.Retrieve(context.Background(), "query")
	require.NoError(t, err)
	require.Len(t, docs, 1)
	require.Equal(t, "doc", docs[0].PageContent)
}

func TestMergerRetriever_ContextCanceled(t *testing.T) {
	retriever := NewMergerRetriever([]Retriever{
		&stubRetriever{},
	}, func(o *MergerRetrieverOptions) {
		o.StopOnFirstError = false
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	docs, err := retriever.Retrieve(ctx, "query")
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, docs)
}

func TestMergerRetriever_MultipleErrorsJoined(t *testing.T) {
	errA := errors.New("errA")
	errB := errors.New("errB")

	retriever := NewMergerRetriever([]Retriever{
		&stubRetriever{err: errA},
		&stubRetriever{err: errB},
	}, func(o *MergerRetrieverOptions) {
		o.StopOnFirstError = false
	})

	docs, err := retriever.Retrieve(context.Background(), "query")
	require.Nil(t, docs)
	require.Error(t, err)
	require.ErrorIs(t, err, errA)
	require.ErrorIs(t, err, errB)
}

type retrieverFunc func(context.Context, string) ([]Document, error)

func (f retrieverFunc) Retrieve(ctx context.Context, query string) ([]Document, error) {
	return f(ctx, query)
}

func TestMergerRetriever_MaxParallel(t *testing.T) {
	var current atomic.Int64
	var max atomic.Int64

	fn := func(ctx context.Context, query string) ([]Document, error) {
		curr := current.Add(1)
		defer current.Add(-1)

		for {
			prev := max.Load()
			if curr <= prev {
				break
			}
			if max.CompareAndSwap(prev, curr) {
				break
			}
		}

		time.Sleep(20 * time.Millisecond)

		return []Document{{PageContent: "doc"}}, nil
	}

	retrievers := []Retriever{
		retrieverFunc(fn),
		retrieverFunc(fn),
		retrieverFunc(fn),
		retrieverFunc(fn),
	}

	r := NewMergerRetriever(
		retrievers,
		func(o *MergerRetrieverOptions) {
			o.MaxParallel = 2
			o.StopOnFirstError = false
		},
	)

	docs, err := r.Retrieve(context.Background(), "query")
	require.NoError(t, err)
	require.Len(t, docs, len(retrievers))
	require.LessOrEqual(t, max.Load(), int64(2))
}

func TestMergerRetriever_StopOnFirstError(t *testing.T) {
	boom := errors.New("boom")
	var called atomic.Int32

	retrievers := []Retriever{
		retrieverFunc(func(ctx context.Context, query string) ([]Document, error) {
			return nil, boom
		}),
		retrieverFunc(func(ctx context.Context, query string) ([]Document, error) {
			called.Add(1)
			return []Document{{PageContent: "doc"}}, nil
		}),
	}

	r := NewMergerRetriever(
		retrievers,
		func(o *MergerRetrieverOptions) {
			o.MaxParallel = 1
			o.StopOnFirstError = true
		},
	)

	docs, err := r.Retrieve(context.Background(), "query")
	require.ErrorIs(t, err, boom)
	require.False(t, errors.Is(err, context.Canceled))
	require.Nil(t, docs)
	require.LessOrEqual(t, called.Load(), int32(1))
}
