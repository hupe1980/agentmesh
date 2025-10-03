package code

import (
	"context"

	"github.com/hupe1980/agentmesh/core"
)

type unsafeLocalExecutor struct{}

// NewUnsafeLocalExecutor constructs an executor that runs code locally without safety guarantees.
func NewUnsafeLocalExecutor() core.CodeExecutor {
	return &unsafeLocalExecutor{}
}

func (e *unsafeLocalExecutor) Execute(
	ctx context.Context,
	reqCtx core.RequestContext,
	input *core.CodeExecutionInput,
) (*core.CodeExecutionResult, error) {
	return &core.CodeExecutionResult{
		Stdout: "",
		Stderr: "",
	}, nil
}

func (e *unsafeLocalExecutor) Close() error {
	return nil
}
