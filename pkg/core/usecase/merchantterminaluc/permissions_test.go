package merchantterminaluc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/merchantterminal"
	"github.com/parstis/bnpl-backend/pkg/core/permission"
)

func TestProvisionPersistsInitialPermissions(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	issued := h.provision()

	terminal := h.terminals.terminals[0]
	stored, err := h.terminals.ListPermissions(h.ctx, terminal.ID)
	require.NoError(t, err)
	require.Equal(t, merchantterminal.InitialPartnerTerminalPermissions(), stored)

	principal, err := h.uc.Authenticate(h.ctx, issued.TerminalID, issued.APIKey)
	require.NoError(t, err)
	require.Equal(t, merchantterminal.InitialPartnerTerminalPermissions(), principal.Permissions)
}

func TestRotationDoesNotChangePermissions(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.provision()
	before := append([]string(nil), h.terminals.permissions[h.terminals.terminals[0].ID]...)

	rotated, err := h.uc.Rotate(h.ctx, rotateInput(h, "rotate-1", nil))
	require.NoError(t, err)
	require.Equal(t, before, h.terminals.permissions[h.terminals.terminals[0].ID])

	principal, err := h.uc.Authenticate(h.ctx, rotated.TerminalID, rotated.APIKey)
	require.NoError(t, err)
	require.Equal(t, before, principal.Permissions)
}

func TestRetireDoesNotChangePermissions(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	first := h.provision()
	before := append([]string(nil), h.terminals.permissions[h.terminals.terminals[0].ID]...)

	grace := time.Hour
	rotated, err := h.uc.Rotate(h.ctx, RotateInput{
		MerchantID:     h.merchantID,
		GracePeriod:    &grace,
		IdempotencyKey: "rotate-1",
	})
	require.NoError(t, err)

	_, err = h.uc.Retire(h.ctx, RetireInput{
		MerchantID:   h.merchantID,
		CredentialID: first.CredentialID,
	})
	require.NoError(t, err)

	require.Equal(t, before, h.terminals.permissions[h.terminals.terminals[0].ID])

	principal, err := h.uc.Authenticate(h.ctx, rotated.TerminalID, rotated.APIKey)
	require.NoError(t, err)
	require.Equal(t, before, principal.Permissions)
}

func TestAuthenticateDropsUnknownStoredPermissions(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	issued := h.provision()
	terminalID := h.terminals.terminals[0].ID

	h.terminals.permissions[terminalID] = append(
		append([]string(nil), h.terminals.permissions[terminalID]...),
		"bnpl:not-a-real-permission:manage:merchant",
	)

	principal, err := h.uc.Authenticate(h.ctx, issued.TerminalID, issued.APIKey)
	require.NoError(t, err)
	require.Equal(t, merchantterminal.InitialPartnerTerminalPermissions(), principal.Permissions)
	require.NotContains(t, principal.Permissions, "bnpl:not-a-real-permission:manage:merchant")
}

func TestAuthenticateWithoutWebhookPermissionOmitsGrant(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	issued := h.provision()
	terminalID := h.terminals.terminals[0].ID

	trimmed := make([]string, 0, len(h.terminals.permissions[terminalID]))
	for _, code := range h.terminals.permissions[terminalID] {
		if code == permission.CheckoutWebhooksManageMerchant.Code {
			continue
		}

		trimmed = append(trimmed, code)
	}

	h.terminals.permissions[terminalID] = trimmed

	principal, err := h.uc.Authenticate(h.ctx, issued.TerminalID, issued.APIKey)
	require.NoError(t, err)
	require.NotContains(t, principal.Permissions, permission.CheckoutWebhooksManageMerchant.Code)
	require.Contains(t, principal.Permissions, permission.CheckoutSessionsCreateMerchant.Code)
}

func TestAuthenticateZeroPermissionsWhenNoneAssigned(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	issued := h.provision()
	h.terminals.permissions[h.terminals.terminals[0].ID] = nil

	principal, err := h.uc.Authenticate(h.ctx, issued.TerminalID, issued.APIKey)
	require.NoError(t, err)
	require.Empty(t, principal.Permissions)
}

func TestAuthenticateDoesNotFallbackToInitialPermissions(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	issued := h.provision()
	h.terminals.permissions[h.terminals.terminals[0].ID] = []string{
		permission.CheckoutSessionsReadMerchant.Code,
	}

	principal, err := h.uc.Authenticate(h.ctx, issued.TerminalID, issued.APIKey)
	require.NoError(t, err)
	require.Equal(t, []string{permission.CheckoutSessionsReadMerchant.Code}, principal.Permissions)
	require.NotEqual(t, merchantterminal.InitialPartnerTerminalPermissions(), principal.Permissions)
}
