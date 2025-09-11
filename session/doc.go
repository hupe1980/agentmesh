// Package session houses concrete implementations of the core.SessionStore.
// The interface itself (and the Session struct) were moved to the core package
// to centralize domain contracts. Keeping only implementations here prevents
// higher level packages (agents, engine) from depending on concrete storage.
//
// Add additional backends (Redis, Postgres, Firestore, etc.) in sub‑packages
// without changing any calling code – only the wiring layer needs to decide
// which implementation to instantiate.
package session
