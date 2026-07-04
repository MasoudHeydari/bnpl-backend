package checkout

import (
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy"
)

// FinancingMode identifies how a checkout policy treats a purchase that
// exceeds currently available customer credit.
type FinancingMode string

const (
	// PartialWithUpfrontFinancing allows the policy to finance the supported
	// portion and require the remainder as upfront cash.
	PartialWithUpfrontFinancing FinancingMode = "partial_with_upfront"
	// FullFinancingOnly requires the full purchase amount to fit within the
	// policy and customer credit limits. No upfront amount is allowed.
	FullFinancingOnly FinancingMode = "full_financing_only"
)

// ActivationMode identifies what business event activates a contract.
type ActivationMode string

const (
	// ActivationAfterInitialPayment keeps the contract payment_pending until
	// the policy-defined initial payment is authoritatively successful in FC.
	ActivationAfterInitialPayment ActivationMode = "after_initial_payment"
	// ActivationAfterMerchantFunding activates the contract without collecting
	// customer cash first. Activation waits for Financial Core merchant-funding
	// (economic recognition) success; it is NOT triggered by local contract
	// creation. The contract stays payment_pending until FC confirms.
	ActivationAfterMerchantFunding ActivationMode = "after_merchant_funding"
)

// ScheduleAnchor identifies the business instant used to finalize installment due dates.
type ScheduleAnchor string

const (
	// ScheduleFromDecision anchors offered due dates to decision time.
	ScheduleFromDecision ScheduleAnchor = "decision"
	// ScheduleFromContractActivation requires due dates to be finalized again
	// from the actual contract activation instant.
	ScheduleFromContractActivation ScheduleAnchor = "contract_activation"
)

// Policy is implemented by every versioned checkout policy. It governs
// checkout-decision eligibility, financing/allocation, installment schedule,
// and the contract activation/economic plan as one business flow.
//
// Implementations must be deterministic, hard-coded, and infrastructure
// independent: no repository access, no external calls, no transactions, no
// environment/config reads, and no time.Now() (current time is always
// supplied explicitly through input values).
type Policy interface {
	// Ref returns this policy version's stable identity.
	Ref() policy.Ref

	// Terms returns this version's explicit, hard-coded business terms. Use
	// cases must read these terms from the policy rather than defining their
	// own fallback or duplicate constants.
	Terms() Terms

	// EvaluateEligibility gates a checkout request using customer/merchant/credit context.
	EvaluateEligibility(input EligibilityInput) EligibilityResult
	// EvaluateFinancing allocates financed and upfront amounts under available credit.
	EvaluateFinancing(input FinancingInput) FinancingResult
	// BuildInstallmentSchedule constructs installment drafts for an approved financing allocation.
	BuildInstallmentSchedule(input ScheduleInput) (ScheduleResult, error)
	// BuildContractPlan describes how the approved allocation becomes an
	// active contract without exposing policy-version checks to application code.
	BuildContractPlan(input ContractPlanInput) (ContractPlan, error)
}

// Terms bundles a checkout policy version's explicit, hard-coded business
// terms so they are testable and referenceable as a single strongly typed
// contract rather than scattered package-level constants.
type Terms struct {
	// InstallmentCount is the fixed number of installments this version
	// finances new decisions into.
	InstallmentCount int

	// DecisionTTL is how long a decision/allocation stays live before expiry.
	DecisionTTL time.Duration

	// MaxFinancedAmountMinor is the maximum principal a single contract may
	// finance under this policy version. It is distinct from customer credit
	// limit and merchant-specific limits even when values happen to match.
	MaxFinancedAmountMinor uint64

	// FinancingMode controls whether a credit shortfall may become upfront cash
	// or requires the checkout to be denied.
	FinancingMode FinancingMode

	// ActivationMode defines which Financial Core success activates the
	// contract: the customer's initial payment, or merchant funding / economic
	// recognition with no customer cash. Local contract creation never
	// activates a contract by itself.
	ActivationMode ActivationMode

	// ScheduleAnchor defines which business time anchors final contractual due dates.
	ScheduleAnchor ScheduleAnchor

	// FirstInstallmentPaymentWindow is how long the customer has to complete an
	// initial activation payment when ActivationMode requires one.
	FirstInstallmentPaymentWindow time.Duration

	// FirstInstallmentDueAfterMonths is the Jalali-month offset from the schedule
	// anchor to installment #1. Later installments increment the offset by one.
	FirstInstallmentDueAfterMonths int

	// GracePeriodDays is the per-installment grace period snapshot persisted
	// at offer time.
	GracePeriodDays uint
}

// EligibilityInput carries customer/merchant/branch/credit-limit context
// needed to gate a checkout request. Now must be supplied by the caller.
type EligibilityInput struct {
	// Now is the evaluation clock supplied by the caller.
	Now time.Time
	// RequestedAmountMinor is the checkout amount requested in minor units.
	RequestedAmountMinor uint64
	// Currency is the currency of RequestedAmountMinor.
	Currency model.Currency
	// BranchRequested reports whether the checkout targets a specific merchant branch.
	BranchRequested bool
	// CustomerProfile is the customer profile under evaluation.
	CustomerProfile model.CustomerProfile
	// MerchantProfile is the merchant profile under evaluation.
	MerchantProfile model.MerchantProfile
	// MerchantBranchProfile is the branch profile when BranchRequested is true.
	MerchantBranchProfile *model.MerchantBranchProfile
	// CreditAccount is the customer's current BNPL credit-capacity snapshot.
	CreditAccount model.CreditAccount
	// HasCreditAccount reports whether CreditAccount is present.
	HasCreditAccount bool
}

// EligibilityResult is the outcome of the eligibility phase.
type EligibilityResult struct {
	// Outcome is the eligibility decision outcome.
	Outcome model.DecisionOutcome
	// ReasonCodes lists machine-readable reasons supporting Outcome.
	ReasonCodes []string
	// Jurisdiction is the jurisdiction applied during eligibility evaluation.
	Jurisdiction model.Jurisdiction
}

// FinancingInput carries available-credit-constrained financing context.
// Eligibility is the prior phase's result; financing seeds its own result
// from it and only proceeds with monetary allocation when eligibility
// approved.
type FinancingInput struct {
	// RequestedAmountMinor is the checkout amount requested in minor units.
	RequestedAmountMinor uint64
	// CreditAccount contains both governed limit and committed principal exposure.
	CreditAccount model.CreditAccount
	// Eligibility is the prior eligibility-phase result.
	Eligibility EligibilityResult
}

// FinancingResult is the outcome of the financing/allocation phase.
type FinancingResult struct {
	// Outcome is the financing decision outcome.
	Outcome model.DecisionOutcome
	// ReasonCodes lists machine-readable reasons supporting Outcome.
	ReasonCodes []string
	// Jurisdiction is the jurisdiction carried forward from eligibility.
	Jurisdiction model.Jurisdiction

	// FinancedAmountMinor is the amount financed across installments.
	FinancedAmountMinor uint64
	// UpfrontAmountMinor is the amount due immediately outside financed principal.
	UpfrontAmountMinor uint64
	// RegularInstallmentAmountMinor is the total amount due on regular installments.
	RegularInstallmentAmountMinor uint64
	// FirstInstallmentAmountMinor is the total amount due on the first installment.
	FirstInstallmentAmountMinor uint64
	// RegularInstallmentPrincipalAmountMinor is the principal portion of regular installments.
	RegularInstallmentPrincipalAmountMinor uint64
	// FirstInstallmentPrincipalAmountMinor is the principal portion of the first installment.
	FirstInstallmentPrincipalAmountMinor uint64
	// FirstInstallmentUpfrontAmountMinor is the upfront portion bundled into the first installment.
	FirstInstallmentUpfrontAmountMinor uint64
	// FirstInstallmentTotalAmountMinor is the full first-installment total including upfront.
	FirstInstallmentTotalAmountMinor uint64
	// TotalFeeAmountMinor is the total fee charged across the schedule.
	TotalFeeAmountMinor uint64
	// TotalPayableAmountMinor is the full customer payable amount.
	TotalPayableAmountMinor uint64
	// AvailableCreditMinor is the customer's available principal capacity at decision time.
	AvailableCreditMinor uint64
}

// ScheduleInput carries data needed to build an installment schedule. Now is
// the provisional schedule anchor used at decision time; policies with
// ScheduleFromContractActivation are finalized again at activation.
type ScheduleInput struct {
	// Now is the schedule anchor supplied by the caller.
	Now time.Time
	// FinancedAmountMinor is the financed principal to split across installments.
	FinancedAmountMinor uint64
	// UpfrontAmountMinor is the upfront amount attached to the first installment.
	UpfrontAmountMinor uint64
	// InstallmentCount is the number of installments from the persisted allocation.
	InstallmentCount int
}

// InstallmentDraft is a single not-yet-persisted installment produced by BuildInstallmentSchedule.
type InstallmentDraft struct {
	// Order is the 1-based installment sequence number.
	Order int
	// PrincipalAmountMinor is the principal portion of this installment.
	PrincipalAmountMinor uint64
	// UpfrontAmountMinor is any upfront amount attached to this installment.
	UpfrontAmountMinor uint64
	// BaseFeeAmountMinor is the base fee portion of this installment.
	BaseFeeAmountMinor uint64
	// TotalAmountMinor is the full amount due for this installment.
	TotalAmountMinor uint64
	// DueAt is when this installment becomes due.
	DueAt time.Time
}

// ScheduleResult is the outcome of the installment schedule phase.
type ScheduleResult struct {
	// Installments are the ordered installment drafts for the offer.
	Installments []InstallmentDraft
}

// ContractPlanInput is the immutable economic snapshot needed to determine
// contract activation behavior for an already-approved checkout.
type ContractPlanInput struct {
	// RequestedAmountMinor is the checkout purchase amount.
	RequestedAmountMinor uint64
	// FinancedAmountMinor is the approved financed principal.
	FinancedAmountMinor uint64
	// UpfrontAmountMinor is the approved upfront cash amount.
	UpfrontAmountMinor uint64
	// FirstInstallmentPrincipalAmountMinor is installment #1 financed principal.
	FirstInstallmentPrincipalAmountMinor uint64
	// FirstInstallmentTotalAmountMinor is installment #1 total including upfront.
	FirstInstallmentTotalAmountMinor uint64
}

// ContractPlan tells application code how to execute contract creation without
// branching on a concrete policy version.
type ContractPlan struct {
	// ActivationMode identifies the business event that activates the contract.
	ActivationMode ActivationMode
	// ScheduleAnchor identifies how final contractual due dates are anchored.
	ScheduleAnchor ScheduleAnchor
	// InitialPaymentRequired reports whether checkout must collect customer cash before activation.
	InitialPaymentRequired bool
	// InitialPaymentAmountMinor is the exact amount required before activation when applicable.
	InitialPaymentAmountMinor uint64
	// ActiveExposureAmountMinor is principal exposure to claim when the contract becomes active.
	ActiveExposureAmountMinor uint64
	// CollectedCashAmountMinor is cash already collected when merchant entitlement is recorded.
	CollectedCashAmountMinor uint64
	// ReceivableAmountMinor is the FC receivable to recognize at activation.
	ReceivableAmountMinor uint64
	// FirstInstallmentPaidOnActivation reports whether activation settles installment #1.
	FirstInstallmentPaidOnActivation bool
}
