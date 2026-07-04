//go:build production

package swaggerrs

import "github.com/gin-gonic/gin"

// RegisterOptions is retained in production so common route wiring stays stable.
// Swagger itself is not compiled into production binaries.
type RegisterOptions struct {
	Enabled bool
}

// Register intentionally does nothing in production builds.
// Signature matches the development Register (gin.IRouter) so routes wiring stays shared.
func Register(_ gin.IRouter, _ RegisterOptions) {}
