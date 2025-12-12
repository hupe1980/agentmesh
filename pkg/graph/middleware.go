package graph

// ChainNodeMiddleware combines multiple node middleware into one.
// Middleware are applied in order, so the first middleware is the outermost layer.
//
// Example:
//
//	import graphmw "github.com/hupe1980/agentmesh/pkg/graph/middleware"
//
//	combined := graph.ChainNodeMiddleware(
//	    graphmw.LoggingMiddleware[message.Message](logger),
//	    graphmw.TimingMiddleware[message.Message](metricsCallback),
//	    graphmw.RecoveryMiddleware[message.Message](panicHandler),
//	)
//	graph.WithNodeMiddleware(combined)
//
// This produces: logging(timing(recovery(node)))
// Execution flows: logging → timing → recovery → node
func ChainNodeMiddleware[O any](middleware ...NodeMiddleware[O]) NodeMiddleware[O] {
	return func(next NodeFunc[O]) NodeFunc[O] {
		// Apply in reverse order so first middleware is outermost
		for i := len(middleware) - 1; i >= 0; i-- {
			next = middleware[i](next)
		}
		return next
	}
}

// ChainRunMiddleware combines multiple run middleware into one.
// Middleware are applied in order, so the first middleware is the outermost layer.
//
// Example:
//
//	combined := graph.ChainRunMiddleware(
//	    inputValidationMiddleware,
//	    outputValidationMiddleware,
//	    loggingMiddleware,
//	)
//	graph.WithRunMiddleware(combined)
//
// This produces: inputValidation(outputValidation(logging(run)))
// Execution flows: inputValidation → outputValidation → logging → run
func ChainRunMiddleware[I, O any](middleware ...RunMiddleware[I, O]) RunMiddleware[I, O] {
	return func(next RunFunc[I, O]) RunFunc[I, O] {
		// Apply in reverse order so first middleware is outermost
		for i := len(middleware) - 1; i >= 0; i-- {
			next = middleware[i](next)
		}
		return next
	}
}
