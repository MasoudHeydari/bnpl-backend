package permission

// Merchant-panel BNPL permissions from the Identity/FC catalog, plus BNPL-local
// checkout integration-config permissions (webhooks, return destinations).
// Integration config is deliberately separate from the ordinary checkout
// transaction permissions so a credential that can create and cancel checkouts
// cannot re-point webhooks or change return URLs.

var (
	MerchantProfileReadMerchant = permission(
		"bnpl:merchant-profile:read:merchant",
		"Read merchant BNPL profile",
		"Allows a merchant operator to read the authenticated merchant BNPL profile.",
	)
	StoresReadMerchant = permission(
		"bnpl:stores:read:merchant",
		"Read merchant BNPL stores",
		"Allows a merchant operator to read stores belonging to the authenticated merchant.",
	)
	BranchesReadMerchant = permission(
		"bnpl:branches:read:merchant",
		"Read merchant BNPL branches",
		"Allows a merchant operator to read branches belonging to the authenticated merchant.",
	)
	CheckoutSessionsCreateMerchant = permission(
		"bnpl:checkout-sessions:create:merchant",
		"Create merchant BNPL checkout sessions",
		"Allows a merchant operator to create BNPL checkout sessions for the authenticated merchant.",
	)
	CheckoutSessionsReadMerchant = permission(
		"bnpl:checkout-sessions:read:merchant",
		"Read merchant BNPL checkout sessions",
		"Allows a merchant operator to read BNPL checkout sessions for the authenticated merchant.",
	)
	CheckoutWebhooksManageMerchant = permission(
		"bnpl:checkout-webhooks:manage:merchant",
		"Manage merchant BNPL checkout webhook",
		"Allows a merchant operator to register or rotate the checkout lifecycle webhook endpoint and signing secret for the authenticated merchant.",
	)
	CheckoutReturnDestinationsManageMerchant = permission(
		"bnpl:checkout-return-destinations:manage:merchant",
		"Manage merchant BNPL checkout return destinations",
		"Allows a merchant operator to register or update browser return destinations for the authenticated merchant.",
	)
	ContractsReadMerchant = permission(
		"bnpl:contracts:read:merchant",
		"Read merchant BNPL contracts",
		"Allows a merchant operator to read BNPL contracts belonging to the authenticated merchant.",
	)
	PaymentsReadMerchant = permission(
		"bnpl:payments:read:merchant",
		"Read merchant BNPL payments",
		"Allows a merchant operator to read BNPL payments related to the authenticated merchant.",
	)
	SettlementsReadMerchant = permission(
		"bnpl:settlements:read:merchant",
		"Read merchant BNPL settlements",
		"Allows a merchant operator to read BNPL settlements for the authenticated merchant.",
	)
	ReportsReadMerchant = permission(
		"bnpl:reports:read:merchant",
		"Read merchant BNPL reports",
		"Allows a merchant operator to read BNPL reports for the authenticated merchant.",
	)
	MerchantTerminalsManageMerchant = permission(
		"bnpl:merchant-terminals:manage:merchant",
		"Manage merchant partner terminals",
		"Allows a merchant operator to get, provision, rotate, and retire the partner terminal and API-key credentials for the authenticated merchant.",
	)
)

var merchantPanel = []Permission{
	MerchantProfileReadMerchant,
	StoresReadMerchant,
	BranchesReadMerchant,
	CheckoutSessionsCreateMerchant,
	CheckoutSessionsReadMerchant,
	CheckoutWebhooksManageMerchant,
	CheckoutReturnDestinationsManageMerchant,
	ContractsReadMerchant,
	PaymentsReadMerchant,
	SettlementsReadMerchant,
	ReportsReadMerchant,
	MerchantTerminalsManageMerchant,
}

// MerchantPanel returns a defensive copy of merchant-panel catalog permissions.
func MerchantPanel() []Permission {
	return clonePermissions(merchantPanel)
}
