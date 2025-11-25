// Package validate provides reusable validation helpers to standardize error
// messages and reduce code duplication across the agentmesh codebase.
//
// This package centralizes common validation patterns such as nil checking,
// empty string validation, and slice length validation into generic, type-safe
// functions that produce consistent error messages.
//
// # Design Goals
//
//   - **DRY Principle:** Eliminate duplicate validation code across packages
//   - **Consistency:** Standardize error message format throughout the codebase
//   - **Type Safety:** Leverage Go generics for compile-time type checking
//   - **Composability:** Enable chaining multiple validations with All()
//   - **Zero Dependencies:** Pure stdlib implementation (except reflect for interfaces)
//
// # Usage Examples
//
// Single validation:
//
//	func NewAgent(model Model) (*Agent, error) {
//	   if err := validate.NotNil(model, "model"); err != nil {
//	       return nil, err
//	   }
//	   return &Agent{model: model}, nil
//	}
//
// Multiple validations:
//
//	func NewRAGAgent(model Model, retriever Retriever) (*Agent, error) {
//	   if err := validate.All(
//	       validate.NotNil(model, "model"),
//	       validate.NotNil(retriever, "retriever"),
//	   ); err != nil {
//	       return nil, err
//	   }
//	   return &Agent{model: model, retriever: retriever}, nil
//	}
//
// Slice validation:
//
//	func AddWorkers(workers []Worker) error {
//	   if err := validate.NotEmptySlice(workers, "workers"); err != nil {
//	       return fmt.Errorf("supervisor: %w", err)
//	   }
//	   return nil
//	}
//
// # Implementation Details
//
// The NotNil function uses reflection to properly handle interface types,
// which can be non-nil but contain nil underlying values. This is necessary
// because Go's interface comparison only checks if the interface itself is nil,
// not whether the concrete value it holds is nil.
//
// Example of the interface nil problem:
//
// var model *ConcreteModel = nil
// var iface Model = model  // iface != nil, but contains nil value
//
//	if iface == nil {
//	   // This branch is NOT taken!
//	}
//
//	if err := validate.NotNil(iface, "model"); err != nil {
//	   // This branch IS taken (correctly detects nil)
//	}
//
// # Performance
//
// NotNil uses reflection which has a small performance cost (~100ns per call).
// This is acceptable for validation code which runs during initialization,
// not in hot paths. For performance-critical paths, consider inline nil checks.
//
// # Testing
//
// The package has 100% test coverage with tests for:
//   - Pointer types (nil and non-nil)
//   - Interface types (nil and non-nil underlying values)
//   - String validation (empty and non-empty)
//   - Slice validation (nil, empty, and non-empty)
//   - Error chaining with All()
//   - Generic type parameters
package validate
