// Package guardrail provides a unified system for content validation and safety checks.
//
// This package is intentionally dependency-free (no imports from agent, tool,
// or message packages) to allow use in any context without cyclic dependencies.
//
// The package provides:
//   - Generic Guardrail[T] interface for any input type
//   - Action enum (Allow, Reject, Raise) for nuanced responses
//   - Result types for both tripwire and 3-action patterns
//   - Built-in implementations (PII, SQL injection, content filter)
//
// External service integrations are provided in subpackages:
//   - pkg/guardrail/openai - OpenAI Moderation API
//   - pkg/guardrail/amazoncomprehend - AWS Comprehend (sentiment, PII)
//
// Integration packages (pkg/tool/middleware, pkg/model/middleware, pkg/agent)
// adapt these generic guardrails to their specific contexts.
//
// # Quick Start
//
// // Create a guardrail
// piiGuardrail := guardrail.NewPIIGuardrail()
//
// // Check content
// result, err := piiGuardrail.Check(ctx, "My SSN is 123-45-6789")
//
//	if err != nil {
//	   log.Fatal(err)
//	}
//
// switch result.Action {
// case guardrail.ActionAllow:
//
//	// Proceed normally
//
// case guardrail.ActionReject:
//
//	// Soft rejection - caller can retry with different content
//	log.Printf("Rejected: %s", result.Message)
//
// case guardrail.ActionRaise:
//
//	   // Hard stop - halt execution
//	   return guardrail.NewTripwireError(result, piiGuardrail.Name())
//	}
//
// # Chaining Guardrails
//
// chain := guardrail.NewChainGuardrail("safety-chain",
//
//	guardrail.NewPIIGuardrail(),
//	guardrail.NewSQLInjectionGuardrail(),
//	guardrail.NewContentFilterGuardrail([]string{"banned", "words"}),
//
// )
//
// result, err := chain.Check(ctx, input)
//
// # Instrumentation
//
// Wrap guardrails with metrics collection:
//
// instrumented := guardrail.Instrument(piiGuardrail, guardrail.LayerTool)
// // Uses metrics.FromContext(ctx) automatically
package guardrail
