package graph

// Chain combines multiple middleware into one.
// Middleware are applied in order, so the first middleware is the outermost layer.
//
// Example:
//
//	import graphmw "github.com/hupe1980/agentmesh/pkg/graph/middleware"
//
//	combined := graph.Chain(
//	    graphmw.LoggingMiddleware[message.Message](logger),
//	    graphmw.TimingMiddleware[message.Message](metricsCallback),
//	    graphmw.RecoveryMiddleware[message.Message](panicHandler),
//	)
//	graph.WithMiddleware(combined)
//
// This produces: logging(timing(recovery(node)))
// Execution flows: logging → timing → recovery → node
func Chain[O any](middleware ...Middleware[O]) Middleware[O] {
	return func(next NodeFunc[O]) NodeFunc[O] {
		// Apply in reverse order so first middleware is outermost
		for i := len(middleware) - 1; i >= 0; i-- {
			next = middleware[i](next)
		}
		return next
	}
}
