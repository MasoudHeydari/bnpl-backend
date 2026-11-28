package permission

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// MerchantWorkspace carries the current Merchant identity and outlet grant.
// It is not a permission or a credential and must not be persisted.
type MerchantWorkspace struct {
	MerchantID model.ExternalMerchantID
	Scope      model.MerchantOutletScope
}

type merchantWorkspaceKey struct{}

// WithMerchantWorkspace attaches a defensive copy of the validated workspace.
func WithMerchantWorkspace(ctx context.Context, workspace MerchantWorkspace) context.Context {
	workspace.Scope.OutletIDs = append([]model.ExternalBranchID(nil), workspace.Scope.OutletIDs...)
	return context.WithValue(ctx, merchantWorkspaceKey{}, workspace)
}

// MerchantWorkspaceFromContext returns the workspace only if it is valid.
func MerchantWorkspaceFromContext(ctx context.Context) (MerchantWorkspace, bool) {
	if ctx == nil {
		return MerchantWorkspace{}, false
	}

	workspace, ok := ctx.Value(merchantWorkspaceKey{}).(MerchantWorkspace)
	if !ok || workspace.MerchantID.IsZero() || !workspace.Scope.IsValid() {
		return MerchantWorkspace{}, false
	}

	workspace.Scope.OutletIDs = append([]model.ExternalBranchID(nil), workspace.Scope.OutletIDs...)
	return workspace, true
}
