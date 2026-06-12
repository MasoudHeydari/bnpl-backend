// Package routes registers HTTP routes on the Gin engine.
// It belongs to the adapter layer and is the composition point where transport
// handlers are wired to use cases and infrastructure dependencies.
package routes

import (
	"context"

	"github.com/gin-gonic/gin"
	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/adapter/config"
	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/core/repo"
)

// Register attaches HTTP routes to e.
// It currently creates only the /api/v1 route group with no handlers.
// Parameters p and c are reserved for future handler wiring.
func Register(ctx context.Context, e *gin.Engine, p repo.Pool, c *config.Config) error {
	_ = ctx
	_ = p
	_ = c
	_ = e.Group("/api/v1")
	return nil
}
