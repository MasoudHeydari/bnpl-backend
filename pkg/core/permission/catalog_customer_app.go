package permission

// Customer-app (self-scoped) BNPL permissions from the Identity/FC catalog,
// plus BNPL-local credit-limits:create:self.

var (
	CheckoutSessionsReadSelf = permission(
		"",
		"Read own BNPL checkout sessions",
		"Allows a customer to read their own BNPL checkout sessions.",
	)
	CheckoutSessionsStartSelf = permission(
		"",
		"Start own BNPL checkout session",
		"Allows a customer to start their own BNPL checkout session.",
	)
	DecisionsCreateSelf = permission(
		"",
		"Create own BNPL decision",
		"Allows a customer to create their own BNPL checkout decision.",
	)
	DecisionsReadSelf = permission(
		"",
		"Read own BNPL decisions",
		"Allows a customer to read their own BNPL checkout decisions.",
	)
	CreditLimitsReadSelf = permission(
		"",
		"Read own BNPL credit limit",
		"Allows a customer to read their own BNPL credit limit summary.",
	)
	CreditLimitsCreateSelf = permission(
		"",
		"Create own BNPL credit limit",
		"Allows a customer to initiate creation of their own BNPL credit limit.",
	)
	ConsentsCreateSelf = permission(
		"",
		"Create own BNPL consents",
		"Allows a customer to create their own BNPL consents.",
	)
	ConsentsReadSelf = permission(
		"",
		"Read own BNPL consents",
		"Allows a customer to read their own BNPL consents.",
	)
	ContractsCreateSelf = permission(
		"",
		"Create own BNPL contract",
		"Allows a customer to create their own BNPL contract from an approved decision.",
	)
	ContractsReadSelf = permission(
		"",
		"Read own BNPL contracts",
		"Allows a customer to read BNPL contracts they own.",
	)
	ContractsSubmitSelf = permission(
		"",
		"Submit own BNPL contract",
		"Allows a customer to submit their own BNPL contract.",
	)
	ContractsCancelSelf = permission(
		"",
		"Cancel own BNPL contract",
		"Allows a customer to cancel an active BNPL contract they own.",
	)
	RepaymentPlansReadSelf = permission(
		"",
		"Read own BNPL repayment plans",
		"Allows a customer to read their own BNPL repayment plans.",
	)
	InstallmentsReadSelf = permission(
		"",
		"Read own BNPL installments",
		"Allows a customer to read BNPL installments they own.",
	)
	PaymentsStartSelf = permission(
		"",
		"Start own BNPL payment",
		"Allows a customer to start repayment of their own BNPL installment.",
	)
	PaymentsReadSelf = permission(
		"",
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
