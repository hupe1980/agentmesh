package testutil

import (
	"context"

	"github.com/hupe1980/agentmesh/core"
)

// MockModel is a lightweight core.Model test double.
// Configure GenerateFunc for custom behavior; otherwise a single final
// text response ("test") is emitted and channels are closed.
type MockModel struct {
	// InfoVal is returned by Info(). If zero, a sane default is used.
	InfoVal core.ModelInfo

	// GenerateFunc, if set, is invoked by Generate. When nil, a default
	// one-shot, non-streaming response is produced.
	GenerateFunc func(ctx context.Context, req *core.ModelRequest) (<-chan *core.ModelResponse, <-chan error)
}

// Generate implements core.Model.
func (m *MockModel) Generate(ctx context.Context, req *core.ModelRequest) (<-chan *core.ModelResponse, <-chan error) {
	if m.GenerateFunc != nil {
		return m.GenerateFunc(ctx, req)
	}

	respCh := make(chan *core.ModelResponse, 1)
	errCh := make(chan error, 1)

	respCh <- &core.ModelResponse{
		Partial:      false,
		Parts:        []core.Part{core.NewPartFromText("test")},
		FinishReason: "stop",
	}

	close(respCh)
	close(errCh)

	return respCh, errCh
}

// Info implements core.Model.
func (m *MockModel) Info() core.ModelInfo {
	if m.InfoVal.Name == "" && m.InfoVal.Provider == "" && !m.InfoVal.SupportsTools {
		return core.ModelInfo{Name: "mock", Provider: "mock", SupportsTools: true}
	}

	return m.InfoVal
}

// Compile-time assertion
var _ core.Model = (*MockModel)(nil)
