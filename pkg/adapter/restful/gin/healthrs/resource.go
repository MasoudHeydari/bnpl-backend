package healthrs

import (
	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/core/usecase/healthcheckuc"
)

// resource exposes liveness and readiness HTTP endpoints.
type resource struct {
	health *healthcheckuc.UseCase
}

// RegisterOptions configures HTTP route registration.
type RegisterOptions struct {
	Enabled bool
}

// Register attaches liveness and readiness health routes when enabled.
//
// gin.IRouter is intentional: these operational endpoints mount on the root
// surface (Engine or an equivalent router), not under the versioned API group.
func Register(router gin.IRouter, health *healthcheckuc.UseCase, opts RegisterOptions) {
	rs := &resource{health: health}

	if !opts.Enabled {
		return
	}

	router.GET("/healthz/live", rs.Live)
	router.GET("/healthz/ready", rs.Ready)
}
