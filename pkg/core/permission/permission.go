// Package permission defines enforceable endpoint permissions and the
// HTTP-independent AuthContext that carries trusted gateway authentication
// metadata through request-scoped context.Context values.
//
// Transport adapters extract trusted gateway authentication headers and attach
// an AuthContext. Use cases and authorization middleware consume that context
// without depending on Gin or net/http.
//
// The permission catalog mirrors the Identity/FC PermissionSeed codes
// (bnpl:<resource>:<action>:<scope>). Name and Description are local catalog
// metadata and are never read from gateway headers.
package permission
