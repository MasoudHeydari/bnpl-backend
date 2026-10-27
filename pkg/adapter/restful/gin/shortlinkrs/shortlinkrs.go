// Package shortlinkrs exposes unauthenticated HTTP redirects for short links.
//
// Routes mount on the root HTTP surface (not under /api/v1) so public short
// URLs such as /pay can reach BNPL without a service-specific prefix.
package shortlinkrs
