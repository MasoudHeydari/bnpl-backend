package routes

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/healthrs"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/healthcheckuc"
)

// Register attaches HTTP routes to e.
func Register(
	ctx context.Context,
	e *gin.Engine,
	healthUseCase *healthcheckuc.UseCase,
	opts RegisterOptions,
) error {
	_ = ctx

	healthrs.Register(e, healthUseCase, opts.Health)

	_ = e.Group("/api/v1")

	return nil
}
