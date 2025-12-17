package graph

// ChainNodeMiddleware combines multiple node middleware into one.
// Middleware are applied in order, so the first middleware is the outermost layer.
//
// Example:
//
//	import graphmw "github.com/hupe1980/agentmesh/pkg/graph/middleware"
//
//	combined := graph.ChainNodeMiddleware(
//	    graphmw.LoggingMiddleware(logger),
//	    graphmw.TimingMiddleware(metricsCallback),
//	    graphmw.RecoveryMiddleware(panicHandler),
//	)
//	graph.WithNodeMiddleware(combined)
//
// This produces: logging(timing(recovery(node)))
// Execution flows: logging → timing → recovery → node
func ChainNodeMiddleware(middleware ...NodeMiddleware) NodeMiddleware {
	return func(next NodeFunc) NodeFunc {
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
func ChainRunMiddleware(middleware ...RunMiddleware) RunMiddleware {
	return func(next RunFunc) RunFunc {
		// Apply in reverse order so first middleware is outermost
		for i := len(middleware) - 1; i >= 0; i-- {
			next = middleware[i](next)
		}
		return next
	}
}
