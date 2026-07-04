package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/header"
	"github.com/parstis/bnpl-backend/pkg/core/trace"
)

// TraceID returns a Gin middleware that propagates request-identification
// headers into the request context and response headers.
//
// Header behavior:
//
//   - X-Trace-Id:
//     Reused from the incoming request when present; otherwise generated.
//     Echoed back in the response.
//
//   - X-Request-Id:
//     Reused from the incoming request when present; otherwise generated.
//     Echoed back in the response.
//
//   - X-Correlation-Id:
//     Reused from the incoming request when present; otherwise generated.
//     Echoed back in the response.
//
// The concrete identifier format is provided by trace.Provider. The middleware
// does not depend on ULID, UUID, or any other infrastructure-level format.
func TraceID(provider trace.Provider) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		traceID := strings.TrimSpace(c.GetHeader(header.TraceID))
		if traceID == "" {
			traceID = provider.NewTraceID()
		}

		ctx = trace.WithTraceID(ctx, traceID)
		c.Header(header.TraceID, traceID)

		requestID := strings.TrimSpace(c.GetHeader(header.RequestID))
		if requestID == "" {
			requestID = provider.NewRequestID()
		}

		ctx = trace.WithRequestID(ctx, requestID)
		c.Header(header.RequestID, requestID)

		correlationID := strings.TrimSpace(c.GetHeader(header.CorrelationID))
		if correlationID == "" {
			correlationID = provider.NewCorrelationID()
		}

		ctx = trace.WithCorrelationID(ctx, correlationID)
		c.Header(header.CorrelationID, correlationID)

		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
