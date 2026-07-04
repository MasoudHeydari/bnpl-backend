package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	coreerrorreporting "github.com/parstis/bnpl-backend/pkg/core/errorreporting"
	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/trace"
)

// PanicRecovery returns the handler used by Gin's CustomRecovery middleware.
//
// Gin owns the actual defer/recover boundary. This handler is invoked after a
// panic has already been recovered. It reports the panic, records an
// operational error, aborts the request with HTTP 500, and deliberately does
// not re-panic.
//
// A panic in one HTTP request must never terminate the process.
//
// Interaction with HTTP logging: this recovery boundary is registered as the
// outermost middleware (see config.Gin.newEngine). When a panic occurs deeper
// in the stack, Go unwinds past AccessLogger and HTTPDebugLogger before this
// handler runs, so those middlewares do not emit their usual completion
// records and do not observe the AbortWithStatus(500) body written here.
// Operators correlating panics should use the "http panic recovered" log and
// error-reporter event from this handler, not the access/debug exchange logs.
func PanicRecovery(
	logger log.Logger,
	reporter coreerrorreporting.Reporter,
) gin.RecoveryFunc {
	return func(c *gin.Context, recovered any) {
		metadata := httpErrorMetadata(
			c,
			"PANIC",
			http.StatusInternalServerError,
		)

		if reporter != nil {
			reporter.CapturePanic(
				requestContext(c),
				recovered,
				metadata,
			)
		}

		if logger != nil {
			logger.With(
				"panic",
				fmt.Sprint(recovered),
				"http_method",
				metadata.Tags["http.method"],
				"http_route",
				metadata.Tags["http.route"],
			).Error("http panic recovered")
		}

		c.AbortWithStatus(http.StatusInternalServerError)
	}
}

func requestContext(c *gin.Context) context.Context {
	if c == nil || c.Request == nil || c.Request.Context() == nil {
		return context.Background()
	}

	return c.Request.Context()
}

func httpErrorMetadata(c *gin.Context, errorCode string, statusCode int) coreerrorreporting.Metadata {
	metadata := coreerrorreporting.Metadata{
		Component: "http",
		Tags: map[string]string{
			"error_code":       errorCode,
			"http.status_code": strconv.Itoa(statusCode),
		},
	}

	if c == nil {
		return metadata
	}

	if c.Request != nil {
		metadata.Tags["http.method"] = c.Request.Method
	}

	if route := c.FullPath(); route != "" {
		metadata.Tags["http.route"] = route
	}

	ctx := requestContext(c)
	metadata.Contexts = map[string]map[string]any{
		"request_ids": {
			"trace_id":       trace.ID(ctx),
			"request_id":     trace.RequestID(ctx),
			"correlation_id": trace.CorrelationID(ctx),
		},
	}

	return metadata
}
