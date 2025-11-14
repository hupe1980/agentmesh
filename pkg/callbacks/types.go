package callbacks

import (
	"context"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/state"
)

// BeforeModelCallback allows interception of model requests prior to execution.
// Returning a non-nil message.Message short-circuits the model invocation and
// uses that message as the final output. Return nil to continue normal flow.
//
// The Writer provides full access to the graph state, including messages
// (via s.MessagesSnapshot()), custom state fields, and the ability to mutate state.
//
// Use cases:
//   - Content safety filtering
//   - Response caching (check cache, return cached response)
//   - Input validation and rewriting
//   - Debug logging and tracing
//   - Policy enforcement (e.g., rate limiting using state)
//   - Stateful guardrails (track conversation context)
//
// Example - Simple cache:
//
//	func CacheCheck(ctx context.Context, s state.Writer) (message.Message, error) {
//	    messages := s.MessagesSnapshot()
//	    key := hashMessages(messages)
//	    if cached := cache.Get(key); cached != nil {
//	        return cached, nil  // Short-circuit
//	    }
//	    return nil, nil  // Continue to model
//	}
//
// Example - Stateful rate limiting:
//
//	func RateLimit(ctx context.Context, s state.Writer) (message.Message, error) {
//	    count := s.Get("request_count").(int)
//	    if count > 10 {
//	        return message.NewAI("Rate limit exceeded"), nil
//	    }
//	    s.Set("request_count", count+1)
//	    return nil, nil  // Continue
//	}
type BeforeModelCallback func(ctx context.Context, s state.Writer) (message.Message, error)

// AfterModelCallback allows post-processing of model responses before they
// are returned to the graph. Returning a non-nil message.Message replaces
// the original response; returning nil keeps the original.
//
// The Writer provides access to conversation history and custom state.
// Callbacks can read state for context-aware transformations or write state
// to track conversation metadata.
//
// Use cases:
//   - Toxicity filtering or content rewriting
//   - Guardrail transformations (e.g., PII redaction)
//   - Response validation
//   - Metrics collection and tracing
//   - Error wrapping or recovery
//   - A/B testing and experimentation
//   - Conversation state tracking
//
// Example - Content filter:
//
//	func FilterToxicity(ctx context.Context, s state.Writer, response message.Message) (message.Message, error) {
//	    if containsToxicity(response.Parts().Text()) {
//	        return message.NewAI("[Content filtered]"), nil
//	    }
//	    return nil, nil  // Keep original
//	}
//
// Example - Track topics:
//
//	func TrackTopics(ctx context.Context, s state.Writer, response message.Message) (message.Message, error) {
//	    topics := extractTopics(response)
//	    existing := s.Get("discussed_topics").([]string)
//	    s.Set("discussed_topics", append(existing, topics...))
//	    return nil, nil  // Keep original
//	}
type AfterModelCallback func(ctx context.Context, s state.Writer, response message.Message) (message.Message, error)

// OnModelErrorCallback handles model invocation failures.
// It may return a fallback message or propagate the error.
//
// Use this to log failures, implement retries, fallback models, or synthesize
// a graceful degradation response. Returning a non-nil message.Message (with
// nil error) replaces the failed call's output and suppresses the original
// error. Returning nil with a nil error leaves the error as-is.
// Returning a non-nil error propagates or transforms the failure.
//
// The Writer allows accessing conversation context and state for intelligent
// error handling, such as checking retry counts or fallback model selection.
//
// Use cases:
//   - Graceful degradation (return fallback response)
//   - Fallback to alternative models based on state
//   - Error transformation and wrapping
//   - Retry coordination (track attempts in state)
//   - Alert triggering
//   - Circuit breaker integration
//   - Error logging and metrics
//
// Example - Fallback model:
//
//	func FallbackModel(ctx context.Context, s state.Writer, err error) (message.Message, error) {
//	    if errors.Is(err, ErrRateLimited) {
//	        messages := s.MessagesSnapshot()
//	        // Use a simpler fallback model
//	        return fallbackModel.Generate(ctx, messages)
//	    }
//	    return nil, err  // Propagate error
//	}
//
// Example - Retry tracking:
//
//	func RetryHandler(ctx context.Context, s state.Writer, err error) (message.Message, error) {
//	    retries := s.Get("retry_count").(int)
//	    if retries < 3 {
//	        s.Set("retry_count", retries+1)
//	        return nil, nil  // Trigger retry
//	    }
//	    return message.NewAI("Service unavailable"), nil  // Fallback
//	}
type OnModelErrorCallback func(ctx context.Context, s state.Writer, err error) (message.Message, error)

// BeforeToolCallback intercepts tool invocations prior to execution.
// Returning a non-nil result skips the actual tool call and uses
// the returned value instead. The result should match the tool's expected output type.
//
// The Writer allows callbacks to enforce policies based on conversation state,
// such as checking permissions, quota limits, or tool usage patterns.
//
// Use cases:
//   - Tool call validation
//   - Parameter sanitization
//   - Access control (e.g., user permissions from state)
//   - Dry-run mode (return mock responses)
//   - Cost estimation and quota tracking
//   - Tool call caching
//
// Example - Access control:
//
//	func CheckPermissions(ctx context.Context, s state.Writer, call message.ToolCall) (any, error) {
//	    userRole := s.Get("user_role").(string)
//	    if !hasPermission(userRole, call.Name) {
//	        return nil, fmt.Errorf("permission denied for tool: %s", call.Name)
//	    }
//	    return nil, nil  // Continue to tool execution
//	}
//
// Example - Tool quota:
//
//	func EnforceQuota(ctx context.Context, s state.Writer, call message.ToolCall) (any, error) {
//	    used := s.Get("tools_used").(int)
//	    if used >= 10 {
//	        return "Tool quota exceeded", nil
//	    }
//	    s.Set("tools_used", used+1)
//	    return nil, nil
//	}
type BeforeToolCallback func(ctx context.Context, s state.Writer, call message.ToolCall) (any, error)

// AfterToolCallback allows inspection and mutation of tool responses.
// Returning a non-nil result replaces the original tool output.
//
// The Writer enables callbacks to track tool usage patterns, store results
// in state for later reference, or make decisions based on conversation history.
//
// Use cases:
//   - Response validation
//   - Result transformation
//   - Metrics and logging
//   - Caching (store result in state)
//   - Retry logic coordination
//   - Output sanitization
//   - Tool result aggregation
//
// Example - Result validation:
//
//	func ValidateToolOutput(ctx context.Context, s state.Writer, call message.ToolCall, result any) (any, error) {
//	    if err := validateOutput(result); err != nil {
//	        return nil, fmt.Errorf("invalid tool output: %w", err)
//	    }
//	    return nil, nil  // Keep original
//	}
//
// Example - Store result:
//
//	func StoreToolResult(ctx context.Context, s state.Writer, call message.ToolCall, result any) (any, error) {
//	    results := s.Get("tool_results").(map[string]any)
//	    results[call.Name] = result
//	    s.Set("tool_results", results)
//	    return nil, nil
//	}
type AfterToolCallback func(ctx context.Context, s state.Writer, call message.ToolCall, result any) (any, error)

// OnToolErrorCallback handles tool invocation failures.
// It may return a fallback result or propagate the error.
//
// The Writer enables intelligent error handling based on conversation state,
// such as checking error counts, implementing backoff strategies, or providing
// context-aware fallback responses.
//
// Use cases:
//   - Graceful degradation (return fallback response)
//   - Error transformation and wrapping
//   - Retry coordination with state tracking
//   - Alert triggering
//   - Circuit breaker integration
//   - Error pattern detection
//
// Example - Fallback:
//
//	func FallbackResponse(ctx context.Context, s state.Writer, call message.ToolCall, err error) (any, error) {
//	    if errors.Is(err, ErrTimeout) {
//	        return "Service temporarily unavailable", nil
//	    }
//	    return nil, err  // Propagate error
//	}
//
// Example - Circuit breaker:
//
//	func CircuitBreaker(ctx context.Context, s state.Writer, call message.ToolCall, err error) (any, error) {
//	    failures := s.Get("tool_failures").(int)
//	    s.Set("tool_failures", failures+1)
//	    if failures >= 5 {
//	        s.Set("circuit_open", true)
//	        return "Service circuit breaker activated", nil
//	    }
//	    return nil, err
//	}
type OnToolErrorCallback func(ctx context.Context, s state.Writer, call message.ToolCall, err error) (any, error)
