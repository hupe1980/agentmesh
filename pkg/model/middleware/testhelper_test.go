package middleware

import (
	"context"

	"github.com/hupe1980/agentmesh/pkg/model"
)

// collect collects all responses and errors from an executor into slices.
func collect(ctx context.Context, exec model.Executor, req *model.Request) ([]*model.Response, []error) {
	var responses []*model.Response
	var errs []error

	for resp, err := range exec.Generate(ctx, req) {
		if err != nil {
			errs = append(errs, err)
		}
		if resp != nil {
			responses = append(responses, resp)
		}
	}

	return responses, errs
}
