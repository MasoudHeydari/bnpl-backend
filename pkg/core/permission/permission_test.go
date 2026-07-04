package permission_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/permission"
)

func TestAuthContextRoundTrip(t *testing.T) {
	t.Parallel()

	original := permission.AuthContext{
		SubjectID: model.SubjectID(uuid.New()),
		TenantID:  model.TenantID(uuid.New()),
		ContextID: model.ContextID(uuid.New()),
		Permissions: []string{
			permission.DecisionsCreateSelf.Code,
			permission.PaymentsStartSelf.Code,
		},
	}

	ctx := permission.WithAuthContext(context.Background(), original)
	got, ok := permission.FromContext(ctx)
	require.True(t, ok)
	require.Equal(t, original.SubjectID, got.SubjectID)
	require.Equal(t, original.TenantID, got.TenantID)
	require.Equal(t, original.ContextID, got.ContextID)
	require.Equal(t, original.Permissions, got.Permissions)

	got.Permissions[0] = "mutated"
	again, ok := permission.FromContext(ctx)
	require.True(t, ok)
	require.Equal(t, permission.DecisionsCreateSelf.Code, again.Permissions[0])

	subjectID, ok := permission.SubjectIDFromContext(ctx)
	require.True(t, ok)
	require.Equal(t, original.SubjectID, subjectID)

	_, ok = permission.FromContext(context.Background())
	require.False(t, ok)
}

func TestHasAnyPermission(t *testing.T) {
	t.Parallel()

	authCtx := permission.AuthContext{
		Permissions: []string{permission.CreditLimitsCreateSelf.Code},
	}
	require.True(t, authCtx.HasAnyPermission(permission.CreditLimitsCreateSelf))
	require.False(t, authCtx.HasAnyPermission(permission.DecisionsCreateSelf))
	require.False(t, authCtx.HasAnyPermission())
}
