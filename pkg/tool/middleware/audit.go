package middleware

import (
	"context"
	"encoding/json"
	"time"

	"github.com/hupe1980/agentmesh/pkg/logging"
	"github.com/hupe1980/agentmesh/pkg/tool"
)

// AuditMiddleware logs tool executions for compliance and debugging.
type AuditMiddleware struct {
	logger logging.Logger
}

// NewAuditMiddleware creates a new audit middleware.
func NewAuditMiddleware(logger logging.Logger) *AuditMiddleware {
	return &AuditMiddleware{
		logger: logger,
	}
}

// Wrap wraps the tool executor with audit logging.
func (m *AuditMiddleware) Wrap(next tool.Executor) tool.Executor {
	return tool.WrapFunc(func(ctx context.Context, calls []tool.Call) ([]tool.ExecutionResult, error) {
		start := time.Now()

		// Log execution start
		if m.logger != nil {
			callSummary := make([]map[string]any, len(calls))
			for i, call := range calls {
				callSummary[i] = map[string]any{
					"id":   call.ID,
					"name": call.Name,
				}
			}
			m.logger.Info("tool execution started", "calls", callSummary, "count", len(calls))
		}

		// Execute
		results, err := next.Execute(ctx, calls)
		duration := time.Since(start)

		// Log execution completion
		if m.logger != nil { //nolint:nestif // acceptable complexity for comprehensive audit logging
			if err != nil {
				m.logger.Error("tool execution failed", "error", err.Error(), "duration", duration.String())
			} else {
				// Log results
				resultSummary := make([]map[string]interface{}, len(results))
				errorCount := 0
				for i, result := range results {
					summary := map[string]interface{}{
						"call_id": calls[i].ID,
						"tool":    calls[i].Name,
						"success": result.Error == nil,
					}
					if result.Error != nil {
						summary["error"] = result.Error.Error()
						errorCount++
					} else {
						// Truncate result for logging
						resultStr := toJSON(result.Result)
						if len(resultStr) > 100 {
							summary["result"] = resultStr[:100] + "..."
						} else {
							summary["result"] = resultStr
						}
					}
					resultSummary[i] = summary
				}

				m.logger.Info("tool execution completed",
					"duration", duration.String(),
					"total", len(results),
					"errors", errorCount,
					"results", resultSummary,
				)
			}
		}

		return results, err
	})
}

// toJSON safely converts a value to JSON string, returning empty string on error.
func toJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(data)
}
