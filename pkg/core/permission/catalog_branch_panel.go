package permission

// Branch-panel BNPL permissions from the Identity/FC catalog.

var (
	BranchProfileReadBranch = permission(
		"bnpl:branch-profile:read:branch",
		"Read branch BNPL profile",
		"Allows a branch operator to read the authenticated branch BNPL profile.",
	)
	CheckoutSessionsCreateBranch = permission(
		"bnpl:checkout-sessions:create:branch",
		"Create branch BNPL checkout sessions",
		"Allows a branch operator to create BNPL checkout sessions for the authenticated branch.",
	)
	CheckoutSessionsReadBranch = permission(
		"bnpl:checkout-sessions:read:branch",
		"Read branch BNPL checkout sessions",
		"Allows a branch operator to read BNPL checkout sessions for the authenticated branch.",
	)
	ContractsReadBranch = permission(
		"bnpl:contracts:read:branch",
		"Read branch BNPL contracts",
		"Allows a branch operator to read BNPL contracts for the authenticated branch.",
	)
	PaymentsReadBranch = permission(
		"bnpl:payments:read:branch",
		"Read branch BNPL payments",
		"Allows a branch operator to read BNPL payments for the authenticated branch.",
	)
)

var branchPanel = []Permission{
	BranchProfileReadBranch,
	CheckoutSessionsCreateBranch,
	CheckoutSessionsReadBranch,
	ContractsReadBranch,
	PaymentsReadBranch,
}

// BranchPanel returns a defensive copy of branch-panel catalog permissions.
func BranchPanel() []Permission {
	return clonePermissions(branchPanel)
}
