// Package gin wraps the upstream Gin HTTP framework so the rest of the codebase
// depends on a small local adapter surface.
//
// It belongs to the adapter layer and provides engine construction plus common
// middleware entrypoints used by the HTTP server bootstrap.
package gin
