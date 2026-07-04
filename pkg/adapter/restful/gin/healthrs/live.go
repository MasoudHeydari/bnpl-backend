package healthrs

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
)

// Live handles GET /healthz/live.
//
//	@Summary		Liveness probe
//	@Description	Reports whether the process is alive. Does not check external dependencies. Public endpoint outside /api/v1.
//	@Tags			Health
//	@Produce		json
//	@Success		200	{object}	Response	"Process is alive"
//	@Failure		500	{object}	serdser.ErrorResponse
//	@Failure		503	{object}	serdser.ErrorResponse	"Process is shutting down after a timed-out graceful shutdown"
//	@Header			200	{string}	X-Trace-Id				"Always set: reuse inbound value or generate"
//	@Header			200	{string}	X-Request-Id			"Always set: reuse inbound value or generate"
//	@Header			200	{string}	X-Correlation-Id		"Always set: reuse inbound value or generate"
//	@Header			500	{string}	X-Trace-Id				"Always set: reuse inbound value or generate"
//	@Header			500	{string}	X-Request-Id			"Always set: reuse inbound value or generate"
//	@Header			500	{string}	X-Correlation-Id		"Always set: reuse inbound value or generate"
//	@Header			503	{string}	X-Trace-Id				"Always set: reuse inbound value or generate"
//	@Header			503	{string}	X-Request-Id			"Always set: reuse inbound value or generate"
//	@Header			503	{string}	X-Correlation-Id		"Always set: reuse inbound value or generate"
//	@Router			/healthz/live [get]
func (rs *resource) Live(c *gin.Context) {
	err := rs.health.Liveness(c.Request.Context())
	if err != nil {
		serdser.SerErr(c, err)
		return
	}

	c.JSON(http.StatusOK, Response{Status: "UP"})
}
