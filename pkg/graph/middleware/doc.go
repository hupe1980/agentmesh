// Package middleware provides reusable middleware for graph node execution.
//
// Available middleware:
//   - LoggingMiddleware: Logs node execution with structured logging
//   - TimingMiddleware: Tracks execution time for performance monitoring
//   - RecoveryMiddleware: Recovers from panics during node execution
//   - ConditionalMiddleware: Applies middleware conditionally based on scope
//   - NodeMiddleware: Applies middleware only to specific nodes
//
// The Chain function from the parent graph package can be used to combine
// multiple middleware into a single middleware:
//
//	import graphmw "github.com/hupe1980/agentmesh/pkg/graph/middleware"
//
//	graph.Chain(
//	    graphmw.LoggingMiddleware[message.Message](logger),
//	    graphmw.TimingMiddleware[message.Message](timingCallback),
//	    graphmw.RecoveryMiddleware[message.Message](panicHandler),
//	)
package middleware
