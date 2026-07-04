package permission

// Admin-panel BNPL permissions from the Identity/FC catalog.

var (
	CustomersReadAny = permission(
		"bnpl:customers:read:any",
		"Read any BNPL customer",
		"Allows an administrator to read BNPL customer profiles.",
	)
	CustomersBlockAny = permission(
		"bnpl:customers:block:any",
		"Block any BNPL customer",
		"Allows an administrator to block BNPL customers.",
	)
	CustomersUnblockAny = permission(
		"bnpl:customers:unblock:any",
		"Unblock any BNPL customer",
		"Allows an administrator to unblock BNPL customers.",
	)
	DecisionsCreateOnBehalf = permission(
		"bnpl:decisions:create:on-behalf",
		"Create BNPL decisions on behalf",
		"Allows an administrator to create BNPL checkout decisions on behalf of a customer.",
	)
	DecisionsReadAny = permission(
		"bnpl:decisions:read:any",
		"Read any BNPL decision",
		"Allows an administrator to read BNPL checkout decisions.",
	)
	DecisionsOverrideAny = permission(
		"bnpl:decisions:override:any",
		"Override any BNPL decision",
		"Allows an administrator to override BNPL checkout decisions.",
	)
	CreditLimitsReadAny = permission(
		"bnpl:credit-limits:read:any",
		"Read any BNPL credit limit",
		"Allows an administrator to read BNPL credit limits.",
	)
	CreditLimitsUpdateAny = permission(
		"bnpl:credit-limits:update:any",
		"Update any BNPL credit limit",
		"Allows an administrator to update BNPL credit limits.",
	)
	ContractsCreateOnBehalf = permission(
		"bnpl:contracts:create:on-behalf",
		"Create BNPL contracts on behalf",
		"Allows an administrator to create BNPL contracts on behalf of a customer.",
	)
	ContractsReadAny = permission(
		"bnpl:contracts:read:any",
		"Read any BNPL contract",
		"Allows an administrator to read BNPL contracts.",
	)
	ContractsSubmitOnBehalf = permission(
		"bnpl:contracts:submit:on-behalf",
		"Submit BNPL contracts on behalf",
		"Allows an administrator to submit BNPL contracts on behalf of a customer.",
	)
	ContractsCancelAny = permission(
		"bnpl:contracts:cancel:any",
		"Cancel any BNPL contract",
		"Allows an administrator to cancel BNPL contracts.",
	)
	ContractsApprove = permission(
		"bnpl:contracts:approve",
		"Approve BNPL contracts",
		"Allows an administrator to approve BNPL contracts.",
	)
	ContractsReject = permission(
		"bnpl:contracts:reject",
		"Reject BNPL contracts",
		"Allows an administrator to reject BNPL contracts.",
	)
	ContractsOverrideAny = permission(
		"bnpl:contracts:override:any",
		"Override any BNPL contract",
		"Allows an administrator to override BNPL contract handling.",
	)
	RepaymentPlansReadAny = permission(
		"bnpl:repayment-plans:read:any",
		"Read any BNPL repayment plan",
		"Allows an administrator to read BNPL repayment plans.",
	)
	InstallmentsReadAny = permission(
		"bnpl:installments:read:any",
		"Read any BNPL installment",
		"Allows an administrator to read BNPL installments.",
	)
	InstallmentsRescheduleAny = permission(
		"bnpl:installments:reschedule:any",
		"Reschedule any BNPL installment",
		"Allows an administrator to reschedule BNPL installments.",
	)
	PaymentsReadAny = permission(
		"bnpl:payments:read:any",
		"Read any BNPL payment",
		"Allows an administrator to read BNPL payments.",
	)
	PaymentsRefundAny = permission(
		"bnpl:payments:refund:any",
		"Refund any BNPL payment",
		"Allows an administrator to refund BNPL payments.",
	)
	MerchantsReadAny = permission(
		"bnpl:merchants:read:any",
		"Read any BNPL merchant",
		"Allows an administrator to read BNPL merchant profiles.",
	)
	StoresReadAny = permission(
		"bnpl:stores:read:any",
		"Read any BNPL store",
		"Allows an administrator to read BNPL stores.",
	)
	BranchesReadAny = permission(
		"bnpl:branches:read:any",
		"Read any BNPL branch",
		"Allows an administrator to read BNPL branches.",
	)
	ReportsReadAny = permission(
		"bnpl:reports:read:any",
		"Read any BNPL report",
		"Allows an administrator to read BNPL reports.",
	)
	ReportsExportAny = permission(
		"bnpl:reports:export:any",
		"Export any BNPL report",
		"Allows an administrator to export BNPL reports.",
	)
	AuditsReadAny = permission(
		"bnpl:audits:read:any",
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
