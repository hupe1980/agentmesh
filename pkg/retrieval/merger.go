package retrieval

import (
	"context"
	"errors"
	"sync"
)

// MergerRetrieverOptions holds the options for the MergerRetriever.
type MergerRetrieverOptions struct {
	// MaxParallel is the maximum number of retrievers to run in parallel.
	// If set to 0, all retrievers will run sequentially.
	MaxParallel uint

	// StopOnFirstError determines whether to stop processing on the first error.
	// If true, the first error encountered will cancel all ongoing retrievals.
	StopOnFirstError bool
}

// WithMergerMaxParallel sets the maximum number of retrievers to run in
// parallel. Use 0 to run sequentially.
func WithMergerMaxParallel(limit uint) func(o *MergerRetrieverOptions) {
	return func(o *MergerRetrieverOptions) {
		o.MaxParallel = limit
	}
}

// WithMergerStopOnFirstError controls whether the merger stops processing when
// the first retriever returns an error.
func WithMergerStopOnFirstError(stop bool) func(o *MergerRetrieverOptions) {
	return func(o *MergerRetrieverOptions) {
		o.StopOnFirstError = stop
	}
}

// MergerRetriever orchestrates multiple retrievers, running them in parallel
// (bounded by MaxParallel) and merging their results into a single slice while
// aggregating any returned errors.
type MergerRetriever struct {
	retrievers []Retriever
	opts       MergerRetrieverOptions
}

// NewMergerRetriever creates a MergerRetriever that fans a query out to the
// provided retrievers. Optional functional options allow callers to tune
// concurrency and error handling behaviour.
func NewMergerRetriever(retrievers []Retriever, optFns ...func(o *MergerRetrieverOptions)) Retriever {
	opts := MergerRetrieverOptions{
		MaxParallel:      4,
		StopOnFirstError: true,
	}

	for _, fn := range optFns {
		fn(&opts)
	}

	return &MergerRetriever{
		retrievers: retrievers,
		opts:       opts,
	}
}

// Retrieve fetches documents from the retrievers in parallel and merges the results.
func (r *MergerRetriever) Retrieve(ctx context.Context, query string) ([]Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	stopOnFirst := r.opts.StopOnFirstError

	type retrieverJob struct {
		idx  int
		retr Retriever
	}

	jobs := make([]retrieverJob, 0, len(r.retrievers))
	for idx, retr := range r.retrievers {
		if retr == nil {
			continue
		}

		jobs = append(jobs, retrieverJob{idx: idx, retr: retr})
	}

	if len(jobs) == 0 {
		return nil, nil
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	type jobResult struct {
		idx  int
		docs []Document
		err  error
	}

	resultsCh := make(chan jobResult, len(jobs))

	var sem chan struct{}
	if r.opts.MaxParallel > 0 && r.opts.MaxParallel < uint(len(jobs)) {
		sem = make(chan struct{}, r.opts.MaxParallel)
	}

	var wg sync.WaitGroup

	for _, job := range jobs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		wg.Add(1)
		go func(j retrieverJob) {
			defer wg.Done()

			if sem != nil {
				select {
				case sem <- struct{}{}:
					defer func() { <-sem }()
				case <-ctx.Done():
					resultsCh <- jobResult{idx: j.idx, err: ctx.Err()}
					return
				}
			}

			if err := ctx.Err(); err != nil {
				resultsCh <- jobResult{idx: j.idx, err: err}
				return
			}

			docs, err := j.retr.Retrieve(ctx, query)
			if stopOnFirst && err != nil {
				cancel()
			}

			resultsCh <- jobResult{idx: j.idx, docs: docs, err: err}
		}(job)
	}

	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	var (
		errs          []error
		stopTriggered bool
	)

	docsByIdx := make(map[int][]Document, len(jobs))

	for res := range resultsCh {
		if res.err != nil {
			if stopOnFirst {
				if !stopTriggered {
					stopTriggered = true
					errs = append(errs, res.err)
					cancel()
				}

				continue
			}

			errs = append(errs, res.err)

			continue
		}

		if len(res.docs) > 0 {
			docsByIdx[res.idx] = res.docs
		}
	}

	var merged []Document
	for _, job := range jobs {
		if docs := docsByIdx[job.idx]; len(docs) > 0 {
			merged = append(merged, docs...)
		}
	}

	joinedErr := errors.Join(errs...)

	if stopTriggered {
		return nil, joinedErr
	}

	if len(merged) == 0 {
		return nil, joinedErr
	}

	return merged, joinedErr
}

// Compile-time check to ensure MergerRetriever implements Retriever.
var _ Retriever = (*MergerRetriever)(nil)
