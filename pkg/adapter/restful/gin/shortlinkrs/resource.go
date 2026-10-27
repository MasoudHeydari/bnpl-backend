package shortlinkrs

import (
	"net/http"
	"strings"

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

	// PublicHost, when non-empty, scopes redirect routes to requests whose Host
	// header matches (without port). This prevents `/:code` from colliding with
	// `/api/v1`, `/swagger`, and `/health` on the API host when Envoy routes the
	// shortener domain to the same Gin engine.
	PublicHost string

	// AliasCodes lists stable public path codes to register explicitly
	// (for example "pay"). Empty means no dedicated alias routes are mounted.
	AliasCodes []string

	// DynamicCodes registers `GET /:code` for generated short-link codes.
	// Prefer enabling this only with PublicHost set.
	DynamicCodes bool
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

	var r gin.IRouter = router
	if host := strings.TrimSpace(opts.PublicHost); host != "" {
		r = router.Group("/", requireHost(host))
	}

	for _, code := range opts.AliasCodes {
		if code == "" {
			continue
		}

		pathCode := code
		r.GET("/"+pathCode, func(c *gin.Context) {
			rs.redirect(c, pathCode)
		})
	}

	if opts.DynamicCodes {
		r.GET("/:code", ResolveHandler(shortLink))
	}
}

func requireHost(expectedHost string) gin.HandlerFunc {
	expected := strings.ToLower(strings.TrimSpace(expectedHost))
	return func(c *gin.Context) {
		host := strings.ToLower(c.Request.Host)
		if i := strings.IndexByte(host, ':'); i >= 0 {
			host = host[:i]
		}

		if host != expected {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}

		c.Next()
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
