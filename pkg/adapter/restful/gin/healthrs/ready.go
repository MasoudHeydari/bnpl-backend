package healthrs

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
)

// Ready handles GET /healthz/ready.
func (rs *resource) Ready(c *gin.Context) {
	err := rs.health.Readiness(c.Request.Context())
	if err != nil {
		serdser.SerErr(c, err)
		return
	}

	c.JSON(http.StatusOK, Response{Status: "UP"})
}
