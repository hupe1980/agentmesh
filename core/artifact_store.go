package core

// ArtifactStore defines the interface for artifact persistence. Implementations
// should be thread-safe and scope artifacts by session identifier. Short method
// names (Save/Get/List/Delete) mirror other store interfaces for consistency.
type ArtifactStore interface {
	Save(sessionID, artifactID string, data []byte) error
	Get(sessionID, artifactID string) ([]byte, error)
	List(sessionID string) ([]string, error)
	Delete(sessionID, artifactID string) error
}
