package core

import "context"

// BeforeModelCallback allows agents to intercept model requests prior to execution.
// Returning a non-nil *ModelResponse short-circuits the model invocation and uses
// that response as the final output. Return nil to continue with the normal flow.
type BeforeModelCallback func(ctx context.Context, cbCtx CallbackContext, req *ModelRequest) (*ModelResponse, error)

// AfterModelCallback allows agents to post-process model responses before they
// are converted into events. Returning a non-nil *ModelResponse replaces the
// original response; returning nil keeps the original.
type AfterModelCallback func(ctx context.Context, cbCtx CallbackContext, res *ModelResponse) (*ModelResponse, error)
