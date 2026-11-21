package permission

// Admin-panel BNPL permissions from the Identity/FC catalog.

var (
	CustomersReadAny = permission(
		"",
		"Read any BNPL customer",
		"Allows an administrator to read BNPL customer profiles.",
	)
	CustomersBlockAny = permission(
		"",
		"Block any BNPL customer",
		"Allows an administrator to block BNPL customers.",
	)
	CustomersUnblockAny = permission(
		"",
		"Unblock any BNPL customer",
		"Allows an administrator to unblock BNPL customers.",
	)
	DecisionsCreateOnBehalf = permission(
		"",
		"Create BNPL decisions on behalf",
		"Allows an administrator to create BNPL checkout decisions on behalf of a customer.",
	)
	DecisionsReadAny = permission(
		"",
		"Read any BNPL decision",
		"Allows an administrator to read BNPL checkout decisions.",
	)
	DecisionsOverrideAny = permission(
		"",
		"Override any BNPL decision",
		"Allows an administrator to override BNPL checkout decisions.",
	)
	CreditLimitsReadAny = permission(
		"",
		"Read any BNPL credit limit",
		"Allows an administrator to read BNPL credit limits.",
	)
	CreditLimitsUpdateAny = permission(
		"",
		"Update any BNPL credit limit",
		"Allows an administrator to update BNPL credit limits.",
	)
	ContractsCreateOnBehalf = permission(
		"",
		"Create BNPL contracts on behalf",
		"Allows an administrator to create BNPL contracts on behalf of a customer.",
	)
	ContractsReadAny = permission(
		"",
		"Read any BNPL contract",
		"Allows an administrator to read BNPL contracts.",
	)
	ContractsSubmitOnBehalf = permission(
		"",
		"Submit BNPL contracts on behalf",
		"Allows an administrator to submit BNPL contracts on behalf of a customer.",
	)
	ContractsCancelAny = permission(
		"",
		"Cancel any BNPL contract",
		"Allows an administrator to cancel BNPL contracts.",
	)
	ContractsApprove = permission(
		"",
		"Approve BNPL contracts",
		"Allows an administrator to approve BNPL contracts.",
	)
	ContractsReject = permission(
		"",
		"Reject BNPL contracts",
		"Allows an administrator to reject BNPL contracts.",
	)
	ContractsOverrideAny = permission(
		"",
		"Override any BNPL contract",
		"Allows an administrator to override BNPL contract handling.",
	)
	RepaymentPlansReadAny = permission(
		"",
		"Read any BNPL repayment plan",
		"Allows an administrator to read BNPL repayment plans.",
	)
	InstallmentsReadAny = permission(
		"",
		"Read any BNPL installment",
		"Allows an administrator to read BNPL installments.",
	)
	InstallmentsRescheduleAny = permission(
		"",
		"Reschedule any BNPL installment",
		"Allows an administrator to reschedule BNPL installments.",
	)
	PaymentsReadAny = permission(
		"",
		"Read any BNPL payment",
		"Allows an administrator to read BNPL payments.",
	)
	PaymentsRefundAny = permission(
		"",
		"Refund any BNPL payment",
		"Allows an administrator to refund BNPL payments.",
	)
	MerchantsReadAny = permission(
		"",
		"Read any BNPL merchant",
		"Allows an administrator to read BNPL merchant profiles.",
	)
	MerchantsCreditProductsUpdateAny = permission(
		"",
		"Update any BNPL merchant credit kind eligibility",
		"Allows an administrator to enable or disable BNPL credit kinds (pay_in_installments, pay_next_month) for a merchant.",
	)
	StoresReadAny = permission(
		"",
		"Read any BNPL store",
		"Allows an administrator to read BNPL stores.",
	)
	BranchesReadAny = permission(
		"",
		"Read any BNPL branch",
		"Allows an administrator to read BNPL branches.",
	)
	ReportsReadAny = permission(
		"",
		"Read any BNPL report",
		"Allows an administrator to read BNPL reports.",
	)
	ReportsExportAny = permission(
		"",
		"Export any BNPL report",
		"Allows an administrator to export BNPL reports.",
	)
	AuditsReadAny = permission(
		"",
		"Read any BNPL audit",
		"Allows an administrator to read BNPL audit records.",
	)
)

var adminPanel = []Permission{
	CustomersReadAny,
	CustomersBlockAny,
	CustomersUnblockAny,
	DecisionsCreateOnBehalf,
	DecisionsReadAny,
	DecisionsOverrideAny,
	CreditLimitsReadAny,
	CreditLimitsUpdateAny,
	ContractsCreateOnBehalf,
	ContractsReadAny,
	ContractsSubmitOnBehalf,
	ContractsCancelAny,
	ContractsApprove,
	ContractsReject,
	ContractsOverrideAny,
	RepaymentPlansReadAny,
	InstallmentsReadAny,
	InstallmentsRescheduleAny,
	PaymentsReadAny,
	PaymentsRefundAny,
	MerchantsReadAny,
	MerchantsCreditProductsUpdateAny,
	StoresReadAny,
	BranchesReadAny,
	ReportsReadAny,
	ReportsExportAny,
	AuditsReadAny,
}

// AdminPanel returns a defensive copy of admin-panel catalog permissions.
func AdminPanel() []Permission {
	return clonePermissions(adminPanel)
}
