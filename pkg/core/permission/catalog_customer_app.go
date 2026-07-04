package permission

// Customer-app (self-scoped) BNPL permissions from the Identity/FC catalog,
// plus BNPL-local credit-limits:create:self.

var (
	CheckoutSessionsReadSelf = permission(
		"bnpl:checkout-sessions:read:self",
		"Read own BNPL checkout sessions",
		"Allows a customer to read their own BNPL checkout sessions.",
	)
	CheckoutSessionsStartSelf = permission(
		"bnpl:checkout-sessions:start:self",
		"Start own BNPL checkout session",
		"Allows a customer to start their own BNPL checkout session.",
	)
	DecisionsCreateSelf = permission(
		"bnpl:decisions:create:self",
		"Create own BNPL decision",
		"Allows a customer to create their own BNPL checkout decision.",
	)
	DecisionsReadSelf = permission(
		"bnpl:decisions:read:self",
		"Read own BNPL decisions",
		"Allows a customer to read their own BNPL checkout decisions.",
	)
	CreditLimitsReadSelf = permission(
		"bnpl:credit-limits:read:self",
		"Read own BNPL credit limit",
		"Allows a customer to read their own BNPL credit limit summary.",
	)
	CreditLimitsCreateSelf = permission(
		"bnpl:credit-limits:create:self",
		"Create own BNPL credit limit",
		"Allows a customer to initiate creation of their own BNPL credit limit.",
	)
	ConsentsCreateSelf = permission(
		"bnpl:consents:create:self",
		"Create own BNPL consents",
		"Allows a customer to create their own BNPL consents.",
	)
	ConsentsReadSelf = permission(
		"bnpl:consents:read:self",
		"Read own BNPL consents",
		"Allows a customer to read their own BNPL consents.",
	)
	ContractsCreateSelf = permission(
		"bnpl:contracts:create:self",
		"Create own BNPL contract",
		"Allows a customer to create their own BNPL contract from an approved decision.",
	)
	ContractsReadSelf = permission(
		"bnpl:contracts:read:self",
		"Read own BNPL contracts",
		"Allows a customer to read BNPL contracts they own.",
	)
	ContractsSubmitSelf = permission(
		"bnpl:contracts:submit:self",
		"Submit own BNPL contract",
		"Allows a customer to submit their own BNPL contract.",
	)
	ContractsCancelSelf = permission(
		"bnpl:contracts:cancel:self",
		"Cancel own BNPL contract",
		"Allows a customer to cancel an active BNPL contract they own.",
	)
	RepaymentPlansReadSelf = permission(
		"bnpl:repayment-plans:read:self",
		"Read own BNPL repayment plans",
		"Allows a customer to read their own BNPL repayment plans.",
	)
	InstallmentsReadSelf = permission(
		"bnpl:installments:read:self",
		"Read own BNPL installments",
		"Allows a customer to read BNPL installments they own.",
	)
	PaymentsStartSelf = permission(
		"bnpl:payments:start:self",
		"Start own BNPL payment",
		"Allows a customer to start repayment of their own BNPL installment.",
	)
	PaymentsReadSelf = permission(
		"bnpl:payments:read:self",
		"Read own BNPL payments",
		"Allows a customer to read their own BNPL payment history.",
	)
)

var customerApp = []Permission{
	CheckoutSessionsReadSelf,
	CheckoutSessionsStartSelf,
	DecisionsCreateSelf,
	DecisionsReadSelf,
	CreditLimitsReadSelf,
	CreditLimitsCreateSelf,
	ConsentsCreateSelf,
	ConsentsReadSelf,
	ContractsCreateSelf,
	ContractsReadSelf,
	ContractsSubmitSelf,
	ContractsCancelSelf,
	RepaymentPlansReadSelf,
	InstallmentsReadSelf,
	PaymentsStartSelf,
	PaymentsReadSelf,
}

// CustomerApp returns a defensive copy of customer-app catalog permissions.
func CustomerApp() []Permission {
	return clonePermissions(customerApp)
}
