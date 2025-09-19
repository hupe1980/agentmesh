package model

import (
	"context"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/core"
	"github.com/stretchr/testify/assert"
)

func TestFuncModel_Generate_SingleResponse(t *testing.T) {
	fm := NewFuncModel(
		func(ctx context.Context, req *core.ModelRequest) (<-chan *core.ModelResponse, <-chan error) {
			out := make(chan *core.ModelResponse, 1)
			errCh := make(chan error, 1)
			go func() {
				defer close(out)
				defer close(errCh)
				out <- &core.ModelResponse{
					Partial:      false,
					Parts:        []core.Part{core.NewPartFromText("hello")},
					FinishReason: "stop",
				}
			}()
			return out, errCh
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	respCh, errCh := fm.Generate(ctx, &core.ModelRequest{})
	resps := collectAllResponses(respCh)
	assert.Len(t, resps, 1)
	assert.False(t, resps[0].Partial)
	assert.Equal(t, "stop", resps[0].FinishReason)
	assert.Len(t, resps[0].Parts, 1)
	if tp, ok := resps[0].Parts[0].(*core.TextPart); assert.True(t, ok) {
		assert.Equal(t, "hello", tp.Text)
	}
	assert.NoError(t, drainError(errCh))
}

func TestFuncModel_Generate_Error(t *testing.T) {
	sentinel := assert.AnError
	fm := NewFuncModel(
		func(ctx context.Context, req *core.ModelRequest) (<-chan *core.ModelResponse, <-chan error) {
			out := make(chan *core.ModelResponse)
			errCh := make(chan error, 1)
			go func() { defer close(out); defer close(errCh); errCh <- sentinel }()
			return out, errCh
		},
	)

	respCh, errCh := fm.Generate(context.Background(), &core.ModelRequest{})
	resps := collectAllResponses(respCh)
	assert.Empty(t, resps)
	assert.ErrorIs(t, drainError(errCh), sentinel)
}

func TestNewFuncModel_PanicsOnNilFunc(t *testing.T) {
	assert.Panics(t, func() {
		NewFuncModel(nil)
	})
}

// collectAllResponses drains a response channel into a slice.
func collectAllResponses(ch <-chan *core.ModelResponse) []*core.ModelResponse {
	// Preallocate with a reasonable default capacity
	out := make([]*core.ModelResponse, 0, 4)
	for r := range ch {
		out = append(out, r)
	}
	return out
}

// drainError returns the single error from a channel or nil.
func drainError(ch <-chan error) error {
	for e := range ch {
		return e
	}
	return nil
}
