//go:build development

package swaggerrs

import (
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	// Register the committed Swaggo-generated Swagger 2.0 document.
	_ "github.com/parstis/bnpl-backend/gen/swagger"
)

// RegisterOptions configures Swagger UI route registration.
type RegisterOptions struct {
	// Enabled controls whether the Swagger UI and specification routes are registered.
	Enabled bool
}

// Register attaches the Swagger UI routes when Enabled is true.
//
// Registration is skipped entirely when Enabled is false,
// allowing documentation to be disabled in production environments.
//
// gin.IRouter is intentional: Swagger mounts on the root surface, not under
// the versioned API RouterGroup used by business resources.
func Register(router gin.IRouter, opts RegisterOptions) {
	if !opts.Enabled {
		return
	}

	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
}
