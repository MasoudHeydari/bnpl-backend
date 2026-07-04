// Package routes registers HTTP routes on the Gin router owned by bootstrap.
//
// It belongs to the adapter layer and is the composition point where transport
// handlers are wired to use cases and infrastructure dependencies. Engine
// ownership and global HTTP server configuration stay in bootstrap; this package
// creates the /api/v1 RouterGroup and passes it to scoped API resources.
package routes
