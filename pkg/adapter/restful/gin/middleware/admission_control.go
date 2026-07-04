package middleware

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
)

// AdmissionControlOptions configures process-local HTTP admission limits.
//
// These limits complement, rather than replace, gateway/Envoy limits. Keeping a
// service-local boundary protects the process from internal callers and gateway
// misconfiguration and prevents overload from turning into unbounded in-memory
// queueing.
type AdmissionControlOptions struct {
	// MaxBodyBytes is the hard maximum request-body size accepted by the REST
	// process. A value <= 0 disables the body-size limit.
	MaxBodyBytes int64

	// MaxInFlight is the maximum number of requests allowed to execute inside
	// the Gin handler chain concurrently. A value <= 0 disables this bulkhead.
	MaxInFlight int
}

// AdmissionControl enforces bounded request bodies and a fail-fast, process-local
// concurrency bulkhead.
//
// The concurrency limiter intentionally does not queue. Once the process reaches
// its configured safe capacity, additional requests receive 503 so upstream load
// balancers can retry elsewhere instead of accumulating goroutines and latency in
// this process.
func AdmissionControl(opts AdmissionControlOptions) gin.HandlerFunc {
	var slots chan struct{}
	if opts.MaxInFlight > 0 {
		slots = make(chan struct{}, opts.MaxInFlight)
	}

	return func(c *gin.Context) {
		if opts.MaxBodyBytes > 0 {
			if c.Request.ContentLength > opts.MaxBodyBytes {
				writePayloadTooLarge(c, errors.New("request content length exceeds configured maximum"))
				return
			}

			if c.Request.Body != nil {
				// MaxBytesReader also protects requests without Content-Length (for
				// example chunked transfer encoding). The binder maps the resulting
				// *http.MaxBytesError to the public 413 contract.
				c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, opts.MaxBodyBytes)
			}
		}

		if slots == nil {
			c.Next()
			return
		}

		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
			c.Next()

		default:
			retryAfter := 1
			c.Header("Retry-After", "1")
			serdser.SerErr(
				c,
				cerr.NewPublic(
					cerr.CodeServiceUnavailable,
					cerr.DefaultMessageKey(cerr.CodeServiceUnavailable),
					errors.New("http admission concurrency limit reached"),
					cerr.Details{RetryAfterSeconds: &retryAfter},
				),
			)
		}
	}
}

func writePayloadTooLarge(c *gin.Context, cause error) {
	serdser.SerErr(
		c,
		cerr.NewPublic(
			cerr.CodePayloadTooLarge,
			cerr.DefaultMessageKey(cerr.CodePayloadTooLarge),
			cause,
			cerr.Details{},
		),
	)
}
