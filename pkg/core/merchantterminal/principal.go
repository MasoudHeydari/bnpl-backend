package merchantterminal

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// MerchantPrincipal is the authenticated identity of an external partner
// request.
type MerchantPrincipal struct {
	// MerchantID is the merchant that owns the authenticated terminal.
	MerchantID model.MerchantID

	// TerminalID is the public terminal identifier that was presented.
	TerminalID model.TerminalID

	// CredentialID identifies the credential that authenticated the request.
	CredentialID model.MerchantTerminalCredentialID

	// CredentialVersion is the version of the authenticating credential.
	CredentialVersion int64

	// Permissions lists the permissions granted to the principal.
	Permissions []string
}

type principalContextKey struct{}

// WithMerchantPrincipal returns a context carrying principal.
func WithMerchantPrincipal(ctx context.Context, principal MerchantPrincipal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal)
}

// MerchantPrincipalFromContext returns the principal stored by
// WithMerchantPrincipal and whether one was present.
func MerchantPrincipalFromContext(ctx context.Context) (MerchantPrincipal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(MerchantPrincipal)

	return principal, ok
}
