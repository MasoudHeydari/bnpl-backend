// Package shortlinkrs exposes unauthenticated HTTP redirects for short links.
//
// Routes mount under /s on the root HTTP surface (not under /api/v1) so public
// short URLs such as /s/pay can reach BNPL without a service-specific prefix.
package shortlinkrs
