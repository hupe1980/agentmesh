// Package artifact contains concrete implementations of the core.ArtifactService.
//
// The canonical ArtifactService interface lives in the core package to avoid
// dependency cycles and keep domain contracts central. Implementation packages
// like this one (in‑memory, cloud object stores, databases, etc.) provide
// storage backends that can be swapped without touching calling code.
//
// Only lightweight implementation specific types should live here. Callers
// should depend on the core interface rather than concrete types so they can
// substitute alternative persistence layers in tests or production.
package artifact
