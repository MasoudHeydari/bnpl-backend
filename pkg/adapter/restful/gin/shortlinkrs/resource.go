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
	// (without port). Required for mounting: without it no public redirect
	// route is registered.
	PublicHost string

	// DynamicCodes registers `GET /s/:code` for generated short-link codes.
	// Only effective when PublicHost is set.
	DynamicCodes bool
}

// Register attaches unauthenticated short-link redirect routes under /s on the
// root HTTP surface when a validated public host is configured.
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

	r := router.Group(shortlinkuc.PublicPathPrefix, requireHost(host))
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
//
//	@Summary		Redirect a public short link to its destination
//	@Description	Resolves an opaque short code and redirects the browser to the stored HTTPS destination with HTTP 302.
//	@Description	Public endpoint outside /api/v1: the short code is the only credential. Gateway auth headers, tenant context, and permissions are neither required nor evaluated.
//	@Description	Mounted under /s on the configured public short-link host. A request whose Host does not match that host is answered with HTTP 404 by the route guard, before any code lookup.
//	@Description	Unknown, revoked, and expired codes return HTTP 404. A blank code returns HTTP 400.
//	@Description	Bad-request and not-found responses are empty-bodied by design, so public codes cannot be confirmed or enumerated through this endpoint.
//	@Description	Resolution responses carry Cache-Control: no-store, Pragma: no-cache, and Referrer-Policy: no-referrer so revocation and destination changes take effect immediately.
//	@Description	Codes therefore do not leak through intermediate caches or Referer headers.
//	@Description	The destination is authoritative only in the Location header. The redirect body is a browser artifact and must not be parsed as a payload.
//	@Tags			Short Links
//	@Param			code	path		string	true	"Opaque short-link code"	minLength(1)
//	@Success		302		{string}	string	"Redirect to the stored destination; Location carries the destination URL"
//	@Failure		400		"Blank short code; the body is empty"
//	@Failure		404		"Code unknown, revoked, or expired, or the request Host is not the configured public short-link host; the body is empty"
//	@Header			302		{string}	Location			"Absolute HTTPS destination URL"
//	@Header			302		{string}	Cache-Control		"Always no-store"
//	@Header			302		{string}	Pragma				"Always no-cache"
//	@Header			302		{string}	Referrer-Policy		"Always no-referrer"
//	@Header			302		{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			302		{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			302		{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			400		{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			400		{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			400		{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			404		{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			404		{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			404		{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Router			/s/{code} [get]
func ResolveHandler(shortLink *shortlinkuc.UseCase) gin.HandlerFunc {
	rs := &resource{shortLink: shortLink}
	return func(c *gin.Context) {
		rs.redirect(c, c.Param("code"))
	}
}
