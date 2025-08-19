package code

// Executor defines the interface for executing code snippets.
type Executor interface {
	// Execute runs the given code snippet and returns the output or an error.
	Execute(code string) (string, error)
}
