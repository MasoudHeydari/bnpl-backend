package healthrs

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
)

// Live handles GET /healthz/live.
func (rs *resource) Live(c *gin.Context) {
	err := rs.health.Liveness(c.Request.Context())
	if err != nil {
		serdser.SerErr(c, err)
		return
	}

	c.JSON(http.StatusOK, Response{Status: "UP"})
}
