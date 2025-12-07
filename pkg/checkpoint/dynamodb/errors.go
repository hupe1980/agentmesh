// Package dynamodb provides sentinel errors for the DynamoDB checkpoint package.
package dynamodb

import "github.com/hupe1980/agentmesh/pkg/checkpoint"

var (
	// ErrNilCheckpoint is an alias for checkpoint.ErrNilCheckpoint.
	ErrNilCheckpoint = checkpoint.ErrNilCheckpoint

	// ErrEmptyRunID is an alias for checkpoint.ErrEmptyRunID.
	ErrEmptyRunID = checkpoint.ErrEmptyRunID

	// ErrNotImplemented is an alias for checkpoint.ErrNotImplemented.
	ErrNotImplemented = checkpoint.ErrNotImplemented
)
