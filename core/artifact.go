package core

import (
	"context"
)

// ArtifactStore persists arbitrary binary artifacts scoped by session.
// Implementations should be thread-safe. Method names mirror other stores.
type ArtifactStore interface {
	// Save persists a binary artifact to the store.
	Save(ctx context.Context, appName, userID, sessionID, fileName string, artifact Part) error

	// Load retrieves a binary artifact from the store.
	Load(ctx context.Context, appName, userID, sessionID, fileName string) (Part, error)

	// ListKeys retrieves all keys for a given session.
	ListKeys(ctx context.Context, appName, userID, sessionID string) ([]string, error)

	// Delete removes a binary artifact from the store.
	Delete(ctx context.Context, appName, userID, sessionID, fileName string) error

	// Close releases any resources held by the store.
	Close() error
}
