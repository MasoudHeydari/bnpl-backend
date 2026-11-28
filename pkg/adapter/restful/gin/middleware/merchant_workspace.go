package middleware

import (
	"encoding/json"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/header"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/permission"
)

// RequireMerchantWorkspace validates the per-request Gateway resolution.
// The listener must be reachable only from the trusted Gateway. This middleware
// is not a substitute for authenticating that upstream connection.
func (AuthContextMiddleware) RequireMerchantWorkspace() gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, ok := permission.FromContext(c.Request.Context()); !ok {
			serdser.SerErr(c, authContextMissingError())
			c.Abort()

			return
		}

		workspace, err := merchantWorkspaceFromHeaders(c)
		if err != nil {
			serdser.SerErr(c, err)
			c.Abort()

			return
		}

		ctx := permission.WithMerchantWorkspace(c.Request.Context(), workspace)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// RequireAllMerchantOutlets denies merchant-wide operations to outlet-only grants.
func (AuthContextMiddleware) RequireAllMerchantOutlets() gin.HandlerFunc {
	return func(c *gin.Context) {
		workspace, ok := permission.MerchantWorkspaceFromContext(c.Request.Context())
		if !ok || !workspace.Scope.IsAllOutlets() {
			serdser.SerErr(c, cerr.Forbidden(fmt.Errorf("middleware.RequireAllMerchantOutlets: merchant-wide scope required")))
			c.Abort()

			return
		}

		c.Next()
	}
}

func merchantWorkspaceFromHeaders(c *gin.Context) (permission.MerchantWorkspace, error) {
	merchantID, err := requireTrustedAuthUUIDHeader(c, header.AuthMerchantID)
	if err != nil {
		return permission.MerchantWorkspace{}, err
	}

	rawMerchantID, _ := singleHeaderValue(c, header.AuthMerchantID)
	if rawMerchantID != merchantID.String() {
		return permission.MerchantWorkspace{}, authHeaderInvalidError(header.AuthMerchantID)
	}

	rawScope, ok := singleHeaderValue(c, header.AuthOutletScope)
	if !ok {
		return permission.MerchantWorkspace{}, authHeaderRequiredError(header.AuthOutletScope)
	}

	rawIDs, ok := singleHeaderValue(c, header.AuthOutletIDs)
	if !ok {
		return permission.MerchantWorkspace{}, authHeaderRequiredError(header.AuthOutletIDs)
	}

	var ids []string
	if err := json.Unmarshal([]byte(rawIDs), &ids); err != nil || ids == nil {
		return permission.MerchantWorkspace{}, authHeaderInvalidError(header.AuthOutletIDs)
	}

	scope := model.MerchantOutletScope{
		Kind:      model.MerchantOutletScopeKind(rawScope),
		OutletIDs: make([]model.ExternalBranchID, 0, len(ids)),
	}
	seen := make(map[uuid.UUID]struct{}, len(ids))
	for _, rawID := range ids {
		id, err := uuid.Parse(rawID)
		if err != nil || id == uuid.Nil || rawID != id.String() {
			return permission.MerchantWorkspace{}, authHeaderInvalidError(header.AuthOutletIDs)
		}

		if _, exists := seen[id]; exists {
			return permission.MerchantWorkspace{}, authHeaderInvalidError(header.AuthOutletIDs)
		}

		seen[id] = struct{}{}
		scope.OutletIDs = append(scope.OutletIDs, model.ExternalBranchID(id))
	}

	if !scope.IsValid() {
		return permission.MerchantWorkspace{}, authHeaderInvalidError(header.AuthOutletScope)
	}

	return permission.MerchantWorkspace{
		MerchantID: model.ExternalMerchantID(merchantID),
		Scope:      scope,
	}, nil
}
