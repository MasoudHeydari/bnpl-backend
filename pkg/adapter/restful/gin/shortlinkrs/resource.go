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
	// PublicHost scopes redirect routes to requests whose Host header matches
	// (without port). Required for safe mounting: without it, `/:code` would
	// collide with `/api/v1`, `/swagger`, and `/health` on the API host.
	PublicHost string

	// DynamicCodes registers `GET /:code` for generated short-link codes.
	// Only effective when PublicHost is set.
	DynamicCodes bool
}

// Register attaches unauthenticated short-link redirect routes on the root
// HTTP surface when a validated public host is configured.
//
// gin.IRouter is intentional: these redirects mount outside /api/v1.
func Register(
	router gin.IRouter,
	shortLink *shortlinkuc.UseCase,
	opts RegisterOptions,
) {
	if shortLink == nil {
		return
	}

	host := strings.TrimSpace(opts.PublicHost)
	if host == "" || !opts.DynamicCodes {
		return
	}

	r := router.Group("/", requireHost(host))
	r.GET("/:code", ResolveHandler(shortLink))
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
	// Public resolve responses must not be cached so revoked/expired codes and
	// destination changes take effect promptly, and short codes are not stored
	// in intermediate caches via redirect Location headers.
	applyResolveHeaders(c)

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

func applyResolveHeaders(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.Header("Referrer-Policy", "no-referrer")
}

// ResolveHandler exposes a reusable redirect handler for dynamic codes when a
// constrained route is registered by wiring.
func ResolveHandler(shortLink *shortlinkuc.UseCase) gin.HandlerFunc {
	rs := &resource{shortLink: shortLink}
	return func(c *gin.Context) {
		rs.redirect(c, c.Param("code"))
	}
}
