package core

import "context"

// CodeExecutionInput contains the code snippet and metadata needed for execution.
type CodeExecutionInput struct {
	Code string `json:"code"`

	Language string `json:"language,omitempty"`
}

// CodeExecutionResult captures stdout and stderr produced by executing code.
type CodeExecutionResult struct {
	Stdout string `json:"stdout"`

	Stderr string `json:"stderr"`
}

// CodeExecutor executes snippets of code and returns the captured output.
type CodeExecutor interface {
	Execute(ctx context.Context, reqCtx RequestContext, input *CodeExecutionInput) (*CodeExecutionResult, error)
	Close() error
}
