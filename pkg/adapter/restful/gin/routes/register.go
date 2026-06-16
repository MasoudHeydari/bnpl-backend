package routes

import (
	"context"

	"github.com/gin-gonic/gin"
)

// Register attaches HTTP routes to e.
// It currently creates only the /api/v1 route group with no handlers.
// Parameters p and c are reserved for future handler wiring.
func Register(ctx context.Context, e *gin.Engine) error {
	_ = ctx
	_ = e.Group("/api/v1")
	return nil
}
