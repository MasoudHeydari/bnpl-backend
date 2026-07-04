package merchantterminal

import "github.com/parstis/bnpl-backend/pkg/core/permission"

// InitialPartnerTerminalPermissions returns the catalog permissions assigned
// when a partner terminal is first provisioned.
//
// This set matches the supported External Partner integration surface under
// /api/v1/partner/** today: checkout create/read/cancel, webhook destination
// management, and return destination management.
// It is used only as the Provision-time seed written to
// merchant_terminal_permissions — Authenticate never falls back to this list.
//
// Terminal provision/rotate/retire remain Merchant Admin Panel operations and
// are never included here.
//
// A fresh slice is returned on every call so callers may retain or modify it.
func InitialPartnerTerminalPermissions() []string {
	granted := []permission.Permission{
		permission.CheckoutSessionsCreateMerchant,
		permission.CheckoutSessionsReadMerchant,
		permission.CheckoutWebhooksManageMerchant,
		permission.CheckoutReturnDestinationsManageMerchant,
	}

	codes := make([]string, 0, len(granted))
	for _, perm := range granted {
		codes = append(codes, perm.String())
	}

	return codes
}
