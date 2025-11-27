// Package middleware provides reusable middleware for model executors.
//
// Available middleware:
//   - CacheMiddleware: Caches model responses to reduce API calls
//   - RetryMiddleware: Retries failed model calls with exponential backoff
//   - RateLimitMiddleware: Rate limits model calls to prevent quota exhaustion
//   - TokenCounterMiddleware: Tracks token usage across model calls
package middleware
