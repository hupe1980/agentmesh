package evaluation

import "github.com/hupe1980/agentmesh/core"

// Invocation represents a single call to the evaluation system.
type Invocation struct {
	UserContent   core.Content
	FinalResponse core.Content
}

// Result represents the outcome of an evaluation.
type Result struct{}

// Evaluator defines the interface for evaluating agent invocations.
// It takes an Invocation and returns a Result or an error.
// Evaluator is responsible for assessing the quality of agent responses.
type Evaluator interface {
	// Evaluate assesses the quality of a single agent invocation.
	Evaluate(invocation Invocation) (*Result, error)
}
