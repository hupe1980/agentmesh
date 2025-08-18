package evaluation

import "github.com/hupe1980/agentmesh/core"

type Invocation struct {
	UserContent   core.Content
	FinalResponse core.Content
}

type Result struct{}

type Evaluator interface {
	Evaluate(invocation Invocation) (*Result, error)
}
