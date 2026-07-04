package healthrs

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
)

// Ready handles GET /healthz/ready.
//
//	@Summary		Readiness probe
//	@Description	Reports whether the process can accept traffic. Fails while draining or when configured dependency checks fail. Public endpoint outside /api/v1.
//	@Tags			Health
//	@Produce		json
//	@Success		200	{object}	Response	"Process is ready"
//	@Failure		500	{object}	serdser.ErrorResponse
//	@Failure		503	{object}	serdser.ErrorResponse	"Process is draining or a dependency check failed"
//	@Header			200	{string}	X-Trace-Id				"Always set: reuse inbound value or generate"
//	@Header			200	{string}	X-Request-Id			"Always set: reuse inbound value or generate"
//	@Header			200	{string}	X-Correlation-Id		"Always set: reuse inbound value or generate"
//	@Header			500	{string}	X-Trace-Id				"Always set: reuse inbound value or generate"
//	@Header			500	{string}	X-Request-Id			"Always set: reuse inbound value or generate"
//	@Header			500	{string}	X-Correlation-Id		"Always set: reuse inbound value or generate"
//	@Header			503	{string}	X-Trace-Id				"Always set: reuse inbound value or generate"
//	@Header			503	{string}	X-Request-Id			"Always set: reuse inbound value or generate"
//	@Header			503	{string}	X-Correlation-Id		"Always set: reuse inbound value or generate"
//	@Router			/healthz/ready [get]
func (rs *resource) Ready(c *gin.Context) {
	err := rs.health.Readiness(c.Request.Context())
	if err != nil {
		serdser.SerErr(c, err)
		return
	}

	c.JSON(http.StatusOK, Response{Status: "UP"})
}
