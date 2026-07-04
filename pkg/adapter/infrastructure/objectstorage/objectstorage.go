// Package objectstorage selects and documents adapter-side object-storage providers.
//
// Provider identifiers here are configuration vocabulary only. Concrete stores
// live under subpackages such as filesystem and noop and implement
// pkg/core/objectstorage.Store.
package objectstorage
