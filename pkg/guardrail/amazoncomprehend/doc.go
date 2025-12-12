// Package amazoncomprehend provides guardrail implementations using AWS Comprehend.
//
// AWS Comprehend offers sentiment analysis, entity detection, and PII detection
// that can be used for content moderation and security.
//
// This package uses interfaces (SentimentDetector, PIIDetector) to allow
// users to provide their own implementations or wrap the AWS SDK client.
//
// # Sentiment Guardrail
//
// detector := myComprehendClient{} // implements SentimentDetector
// g := amazoncomprehend.NewSentiment(detector,
//
//	amazoncomprehend.WithBlockNegative(0.8),
//
// )
//
// # PII Detection Guardrail
//
// detector := myComprehendClient{} // implements PIIDetector
// g := amazoncomprehend.NewPII(detector,
//
//	amazoncomprehend.WithPIIAction(guardrail.ActionRaise),
//	amazoncomprehend.WithBlockedPIITypes("SSN", "CREDIT_DEBIT_NUMBER"),
//
// )
package amazoncomprehend
