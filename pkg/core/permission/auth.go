package permission

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// AuthContext holds trusted authentication metadata for one request.
//
// SubjectID is the external stable identity issued by Identity Service and
// received through x-auth-subject-id. It maps to customer_profiles.identity_subject_id.
// It is never customer_profiles.id (the internal BNPL profile primary key).
//
// Named AuthContext (rather than Context) so call sites read clearly as
// authentication metadata and do not collide with context.Context.
type AuthContext struct {
	// SubjectID is the Identity Service subject UUID from x-auth-subject-id.
	SubjectID model.SubjectID

	// TenantID identifies the authenticated tenant scope for the request.
	TenantID model.TenantID

	// ContextID identifies the authenticated security context for the request.
	ContextID model.ContextID

	// Permissions lists wire permission codes granted to the authenticated
	// subject by the gateway. Values are opaque strings (typically catalog
	// Codes); Name/Description metadata is never carried on the wire.
	Permissions []string
}

// ClonePermissions returns a defensive copy of Permissions.
func (a AuthContext) ClonePermissions() []string {
	if len(a.Permissions) == 0 {
		return []string{}
	}

	out := make([]string, len(a.Permissions))
	copy(out, a.Permissions)
	return out
}

// HasAnyPermission reports whether AuthContext includes at least one expected
// catalog permission, matching on Permission.Code against granted wire strings.
func (a AuthContext) HasAnyPermission(expected ...Permission) bool {
	if len(expected) == 0 {
		return false
	}

	held := make(map[string]struct{}, len(a.Permissions))
	for _, code := range a.Permissions {
		held[code] = struct{}{}
	}

	for _, want := range expected {
		if _, ok := held[want.Code]; ok {
			return true
		}
	}

	return false
}

type authContextKey struct{}

// WithAuthContext returns a child context carrying auth.
func WithAuthContext(ctx context.Context, authCtx AuthContext) context.Context {
	cloned := authCtx
	cloned.Permissions = authCtx.ClonePermissions()
	return context.WithValue(ctx, authContextKey{}, cloned)
}

// FromContext returns the AuthContext stored in ctx.
//
// ok is false when ctx is nil or no AuthContext has been attached.
func FromContext(ctx context.Context) (AuthContext, bool) {
	if ctx == nil {
		return AuthContext{}, false
	}

	authCtx, ok := ctx.Value(authContextKey{}).(AuthContext)
	if !ok {
		return AuthContext{}, false
	}

	authCtx.Permissions = authCtx.ClonePermissions()
	return authCtx, true
}

// SubjectIDFromContext returns the authenticated subject ID when present.
func SubjectIDFromContext(ctx context.Context) (model.SubjectID, bool) {
	authCtx, ok := FromContext(ctx)
	if !ok {
		return model.SubjectID{}, false
	}

	return authCtx.SubjectID, true
}

// TenantIDFromContext returns the authenticated tenant ID when present.
func TenantIDFromContext(ctx context.Context) (model.TenantID, bool) {
	authCtx, ok := FromContext(ctx)
	if !ok {
		return model.TenantID{}, false
	}

	return authCtx.TenantID, true
}

// ContextIDFromContext returns the authenticated business context ID when present.
func ContextIDFromContext(ctx context.Context) (model.ContextID, bool) {
	authCtx, ok := FromContext(ctx)
	if !ok {
		return model.ContextID{}, false
	}

	return authCtx.ContextID, true
}
