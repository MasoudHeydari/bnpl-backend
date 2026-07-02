package model

// Checkout decision reason codes are stable machine-readable identifiers.
const (
	DecisionReasonApprovedFullFinancing                      = "APPROVED_FULL_FINANCING"
	DecisionReasonPartialApprovalRequiresFirstInstallmentPay = "PARTIAL_APPROVAL_REQUIRES_FIRST_INSTALLMENT_PAYMENT"

	DecisionReasonBorrowerProfileNotActive              = "BORROWER_PROFILE_NOT_ACTIVE"
	DecisionReasonIdentityVerificationRequired          = "IDENTITY_VERIFICATION_REQUIRED"
	DecisionReasonIdentityVerificationNotVerified       = "IDENTITY_VERIFICATION_NOT_VERIFIED"
	DecisionReasonIdentityVerificationExpired           = "IDENTITY_VERIFICATION_EXPIRED"
	DecisionReasonMerchantProfileNotActive              = "MERCHANT_PROFILE_NOT_ACTIVE"
	DecisionReasonMerchantBranchProfileRequired         = "MERCHANT_BRANCH_PROFILE_REQUIRED"
	DecisionReasonMerchantBranchProfileNotActive        = "MERCHANT_BRANCH_PROFILE_NOT_ACTIVE"
	DecisionReasonMerchantBranchDoesNotBelongToMerchant = "MERCHANT_BRANCH_DOES_NOT_BELONG_TO_MERCHANT"
	DecisionReasonCurrencyNotSupported                  = "CURRENCY_NOT_SUPPORTED"
	DecisionReasonMerchantAmountCurrencyMismatch        = "MERCHANT_AMOUNT_CURRENCY_MISMATCH"
	DecisionReasonCreditLimitMissing                    = "CREDIT_LIMIT_MISSING"
	DecisionReasonCreditLimitNotActive                  = "CREDIT_LIMIT_NOT_ACTIVE"
	DecisionReasonCreditLimitExpired                    = "CREDIT_LIMIT_EXPIRED"
	DecisionReasonNoAvailableCredit                     = "NO_AVAILABLE_CREDIT"
	DecisionReasonRequestedAmountExceedsMerchantLimit   = "REQUESTED_AMOUNT_EXCEEDS_MERCHANT_LIMIT"
	DecisionReasonInstallmentCountNotSupported          = "INSTALLMENT_COUNT_NOT_SUPPORTED"
	DecisionReasonJurisdictionMismatch                  = "JURISDICTION_MISMATCH"
)
