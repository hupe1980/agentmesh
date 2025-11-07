package callbacks

import (
	"context"

	"github.com/hupe1980/agentmesh/pkg/message"
)

// BeforeModelCallback allows interception of model requests prior to execution.
// Returning a non-nil message.Message short-circuits the model invocation and
// uses that message as the final output. Return nil to continue normal flow.
//
// The messages slice is mutable - callbacks can modify it before passing to the model.
//
// Use cases:
//   - Content safety filtering
//   - Response caching (check cache, return cached response)
//   - Input validation and rewriting
//   - Debug logging and tracing
//   - Policy enforcement (e.g., rate limiting)
//
// Example - Simple cache:
//
//	func CacheCheck(ctx context.Context, messages []message.Message) (message.Message, error) {
//	    key := hashMessages(messages)
//	    if cached := cache.Get(key); cached != nil {
//	        return cached, nil  // Short-circuit
//	    }
//	    return nil, nil  // Continue to model
//	}
type BeforeModelCallback func(ctx context.Context, messages []message.Message) (message.Message, error)

// AfterModelCallback allows post-processing of model responses before they
// are returned to the graph. Returning a non-nil message.Message replaces
// the original response; returning nil keeps the original.
//
// Use cases:
//   - Toxicity filtering or content rewriting
//   - Guardrail transformations (e.g., PII redaction)
//   - Response validation
//   - Metrics collection and tracing
//   - Error wrapping or recovery
//   - A/B testing and experimentation
//
// Example - Content filter:
//
//	func FilterToxicity(ctx context.Context, messages []message.Message, response message.Message) (message.Message, error) {
//	    if containsToxicity(response.Parts().Text()) {
//	        return message.NewAIMessage("[Content filtered]"), nil
//	    }
//	    return nil, nil  // Keep original
//	}
type AfterModelCallback func(ctx context.Context, messages []message.Message, response message.Message) (message.Message, error)

// OnModelErrorCallback handles model invocation failures.
// It may return a fallback message or propagate the error.
//
// Use this to log failures, implement retries, fallback models, or synthesize
// a graceful degradation response. Returning a non-nil message.Message (with
// nil error) replaces the failed call's output and suppresses the original
// error. Returning nil with a nil error leaves the error as-is.
// Returning a non-nil error propagates or transforms the failure.
//
// Use cases:
//   - Graceful degradation (return fallback response)
//   - Fallback to alternative models
//   - Error transformation and wrapping
//   - Retry coordination
//   - Alert triggering
//   - Circuit breaker integration
//   - Error logging and metrics
//
// Example - Fallback model:
//
//	func FallbackModel(ctx context.Context, messages []message.Message, err error) (message.Message, error) {
//	    if errors.Is(err, ErrRateLimited) {
//	        // Use a simpler fallback model
//	        return fallbackModel.Generate(ctx, messages)
//	    }
//	    return nil, err  // Propagate error
//	}
type OnModelErrorCallback func(ctx context.Context, messages []message.Message, err error) (message.Message, error)

// BeforeToolCallback intercepts tool invocations prior to execution.
// Returning a non-nil result skips the actual tool call and uses
// the returned value instead. The result should match the tool's expected output type.
//
// Use cases:
//   - Tool call validation
//   - Parameter sanitization
//   - Access control (e.g., user permissions)
//   - Dry-run mode (return mock responses)
//   - Cost estimation
//   - Tool call caching
//
// Example - Access control:
//
//	func CheckPermissions(ctx context.Context, call message.ToolCall) (any, error) {
//	    if !hasPermission(ctx, call.Name) {
//	        return nil, fmt.Errorf("permission denied for tool: %s", call.Name)
//	    }
//	    return nil, nil  // Continue to tool execution
//	}
type BeforeToolCallback func(ctx context.Context, call message.ToolCall) (any, error)

// AfterToolCallback allows inspection and mutation of tool responses.
// Returning a non-nil result replaces the original tool output.
//
// Use cases:
//   - Response validation
//   - Result transformation
//   - Metrics and logging
//   - Caching (store result)
//   - Retry logic coordination
//   - Output sanitization
//
// Example - Result validation:
//
//	func ValidateToolOutput(ctx context.Context, call message.ToolCall, result any) (any, error) {
//	    if err := validateOutput(result); err != nil {
//	        return nil, fmt.Errorf("invalid tool output: %w", err)
//	    }
//	    return nil, nil  // Keep original
//	}
type AfterToolCallback func(ctx context.Context, call message.ToolCall, result any) (any, error)

// OnToolErrorCallback handles tool invocation failures.
// It may return a fallback result or propagate the error.
//
// Use cases:
//   - Graceful degradation (return fallback response)
//   - Error transformation and wrapping
//   - Retry coordination
//   - Alert triggering
//   - Circuit breaker integration
//
// Example - Fallback:
//
//	func FallbackResponse(ctx context.Context, call message.ToolCall, err error) (any, error) {
//	    if errors.Is(err, ErrTimeout) {
//	        return "Service temporarily unavailable", nil
//	    }
//	    return nil, err  // Propagate error
//	}
type OnToolErrorCallback func(ctx context.Context, call message.ToolCall, err error) (any, error)
