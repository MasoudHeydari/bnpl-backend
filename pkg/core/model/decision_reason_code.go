package model

// Checkout decision reason codes are stable machine-readable identifiers.
const (
	ApprovedFullFinancingDecisionReason                      = "APPROVED_FULL_FINANCING"
	PartialApprovalRequiresFirstInstallmentPayDecisionReason = "PARTIAL_APPROVAL_REQUIRES_FIRST_INSTALLMENT_PAYMENT"

	CustomerProfileNotActiveDecisionReason                  = "CUSTOMER_PROFILE_NOT_ACTIVE"
	MerchantProfileNotActiveDecisionReason                  = "MERCHANT_PROFILE_NOT_ACTIVE"
	MerchantBranchProfileRequiredDecisionReason             = "MERCHANT_BRANCH_PROFILE_REQUIRED"
	MerchantBranchProfileNotActiveDecisionReason            = "MERCHANT_BRANCH_PROFILE_NOT_ACTIVE"
	MerchantBranchDoesNotBelongToMerchantDecisionReason     = "MERCHANT_BRANCH_DOES_NOT_BELONG_TO_MERCHANT"
	CurrencyNotSupportedDecisionReason                      = "CURRENCY_NOT_SUPPORTED"
	MerchantAmountCurrencyMismatchDecisionReason            = "MERCHANT_AMOUNT_CURRENCY_MISMATCH"
	CreditLimitMissingDecisionReason                        = "CREDIT_LIMIT_MISSING"
	CreditLimitNotActiveDecisionReason                      = "CREDIT_LIMIT_NOT_ACTIVE"
	CreditLimitExpiredDecisionReason                        = "CREDIT_LIMIT_EXPIRED" //nolint:gosec // G101: decision reason code identifier, not a credential
	NoAvailableCreditDecisionReason                         = "NO_AVAILABLE_CREDIT"
	RequestedAmountExceedsMerchantLimitDecisionReason       = "REQUESTED_AMOUNT_EXCEEDS_MERCHANT_LIMIT"
	RequestedAmountExceedsSupportedRangeDecisionReason      = "REQUESTED_AMOUNT_EXCEEDS_SUPPORTED_RANGE"
	InstallmentCountNotSupportedDecisionReason              = "INSTALLMENT_COUNT_NOT_SUPPORTED"
	JurisdictionMismatchDecisionReason                      = "JURISDICTION_MISMATCH"
	FinancedAmountTooSmallForInstallmentCountDecisionReason = "FINANCED_AMOUNT_TOO_SMALL_FOR_INSTALLMENT_COUNT"

	// CollectionsGracePeriodExceededDecisionReason is used when at least one
	// outstanding installment has crossed its contractual grace boundary.
	CollectionsGracePeriodExceededDecisionReason = "COLLECTIONS_GRACE_PERIOD_EXCEEDED"

	// CollectionsDueInstallmentsSettledDecisionReason is used when every
	// installment whose contractual due date has arrived is fully settled.
	CollectionsDueInstallmentsSettledDecisionReason = "COLLECTIONS_DUE_INSTALLMENTS_SETTLED"

	// CollectionsDefaultBlocksNewCreditDecisionReason denies new financing while
	// the customer is effectively in collections default.
	CollectionsDefaultBlocksNewCreditDecisionReason = "COLLECTIONS_DEFAULT_BLOCKS_NEW_CREDIT"
)
