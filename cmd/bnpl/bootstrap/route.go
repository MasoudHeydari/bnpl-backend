package bootstrap

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/routes"
)

// RegisterRoutes registers the HTTP routes for App.
func (a *App) RegisterRoutes(
	ctx context.Context,
	e *gin.Engine,
) error {
	return routes.Register(
		ctx,
		e,
	)
}
