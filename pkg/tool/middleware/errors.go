// Package middleware provides sentinel errors for the tool middleware package.
package middleware

import "errors"

var (
	// ErrCircuitBreakerOpen is returned when a circuit breaker is open.
	ErrCircuitBreakerOpen = errors.New("tool/middleware: circuit breaker is open")
)
