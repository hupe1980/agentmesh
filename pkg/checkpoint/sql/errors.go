// Package sql provides sentinel errors for the SQL checkpoint package.
package sql

import "github.com/hupe1980/agentmesh/pkg/checkpoint"

var (
	// ErrNilCheckpoint is an alias for checkpoint.ErrNilCheckpoint.
	ErrNilCheckpoint = checkpoint.ErrNilCheckpoint

	// ErrEmptyRunID is an alias for checkpoint.ErrEmptyRunID.
	ErrEmptyRunID = checkpoint.ErrEmptyRunID

	// ErrDatabaseRequired is an alias for checkpoint.ErrDatabaseRequired.
	ErrDatabaseRequired = checkpoint.ErrDatabaseRequired
)
