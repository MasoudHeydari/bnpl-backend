package routes

import (
	"context"

	"github.com/gin-gonic/gin"
	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/adapter/config"
	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/core/repo"
)

func Register(ctx context.Context, e *gin.Engine, p repo.Pool, c *config.Config) error {
	_ = e.Group("/api/v1")
	return nil
}
