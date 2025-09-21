package core

import "context"

// Credential represents an authentication credential used for various authentication schemes.
type Credential interface {
	isCredential()
}

// BasicAuthCredential represents credentials for HTTP Basic Authentication.
type BasicAuthCredential struct {
	Username string
	Password string
}

func (c *BasicAuthCredential) isCredential() {}

// BearerTokenCredential represents credentials for Bearer Token Authentication.
type BearerTokenCredential struct {
	Token string
}

func (c *BearerTokenCredential) isCredential() {}

// APIKeyCredential represents credentials for API Key Authentication.
type APIKeyCredential struct {
	Key   string
	Value string
	In    string // "header" or "query"
}

func (c *APIKeyCredential) isCredential() {}

// NoAuthCredential represents the absence of authentication credentials.
type NoAuthCredential struct{}

func (c *NoAuthCredential) isCredential() {}

// CredentialStore defines the interface for storing and retrieving authentication credentials.
type CredentialStore interface {
	// Save persists the credentials.
	Save(ctx context.Context, cbCtx CallbackContext) error

	// Load retrieves the credentials.
	Load(ctx context.Context, cbCtx CallbackContext) (Credential, error)

	// Close releases any resources held by the store.
	Close() error
}
