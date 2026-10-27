package shortlinkrs

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/shortlinkuc"
)

type resource struct {
	shortLink *shortlinkuc.UseCase
}

// RegisterOptions configures short-link HTTP route registration.
type RegisterOptions struct {
	// Enabled controls whether public redirect routes are mounted.
	Enabled bool

	// AliasCodes lists stable public path codes to register explicitly
	// (for example "pay"). Empty means no alias routes are mounted; callers
	// that need generated codes can still register a constrained path later.
	AliasCodes []string
}

// Register attaches unauthenticated short-link redirect routes on the root
// HTTP surface when enabled.
//
// gin.IRouter is intentional: these redirects mount outside /api/v1.
func Register(
	router gin.IRouter,
	shortLink *shortlinkuc.UseCase,
	opts RegisterOptions,
) {
	if !opts.Enabled || shortLink == nil {
		return
	}

	rs := &resource{shortLink: shortLink}
	for _, code := range opts.AliasCodes {
		if code == "" {
			continue
		}

		pathCode := code
		router.GET("/"+pathCode, func(c *gin.Context) {
			rs.redirect(c, pathCode)
		})
	}
}

func (rs *resource) redirect(c *gin.Context, code string) {
	out, err := rs.shortLink.Resolve(c.Request.Context(), code)
	if err != nil {
		status := http.StatusNotFound
		if cerr.IsCode(err, cerr.CodeInvalidArgument) {
			status = http.StatusBadRequest
		}

		_ = c.Error(err)
		c.AbortWithStatus(status)
		return
	}

	c.Redirect(http.StatusFound, out.DestinationURL)
}

// ResolveHandler exposes a reusable redirect handler for dynamic codes when a
// constrained route is registered by wiring.
func ResolveHandler(shortLink *shortlinkuc.UseCase) gin.HandlerFunc {
	rs := &resource{shortLink: shortLink}
	return func(c *gin.Context) {
		rs.redirect(c, c.Param("code"))
	}
}
