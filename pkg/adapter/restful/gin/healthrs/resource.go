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

func Register(e *gin.Engine, health *healthcheckuc.UseCase, opts RegisterOptions) {
	rs := &resource{health: health}

	if !opts.Enabled {
		return
	}

	e.GET("/healthz/live", rs.Live)
	e.GET("/healthz/ready", rs.Ready)
}
