// Package cache provides sentinel errors for the cache package.
package cache

import "errors"

var (
	// ErrMissingResponseData is returned when response data is missing.
	ErrMissingResponseData = errors.New("cache: missing response data")
)
