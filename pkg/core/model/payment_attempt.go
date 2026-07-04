package model

import (
	"database/sql/driver"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// PaymentAttemptID uniquely identifies a local PaymentAttempt.
type PaymentAttemptID uuid.UUID

// ParsePaymentAttemptID parses a PaymentAttemptID from its canonical string form.
func ParsePaymentAttemptID(value string) (PaymentAttemptID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return PaymentAttemptID{}, fmt.Errorf("parse payment attempt id: %w", err)
	}

	return PaymentAttemptID(id), nil
}

func (id PaymentAttemptID) UUID() uuid.UUID { return uuid.UUID(id) }

func (id PaymentAttemptID) String() string { return uuid.UUID(id).String() }

func (id PaymentAttemptID) IsZero() bool { return uuid.UUID(id) == uuid.Nil }

func (id PaymentAttemptID) MarshalText() ([]byte, error) { return uuid.UUID(id).MarshalText() }

func (id *PaymentAttemptID) UnmarshalText(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data); err != nil {
		return err
	}

	*id = PaymentAttemptID(u)

	return nil
}

func (id *PaymentAttemptID) Scan(src any) error {
	var u uuid.UUID
	if err := u.Scan(src); err != nil {
		return err
	}

	*id = PaymentAttemptID(u)

	return nil
}

func (id PaymentAttemptID) Value() (driver.Value, error) { return uuid.UUID(id).Value() }

// PaymentAttemptState identifies the local BNPL repayment orchestration state.
type PaymentAttemptState string

// Known local payment attempt orchestration states.
const (
	CreatedPaymentAttemptState                PaymentAttemptState = "created"
	SubmittedPaymentAttemptState              PaymentAttemptState = "submitted"
	SucceededPaymentAttemptState              PaymentAttemptState = "succeeded"
	FailedPaymentAttemptState                 PaymentAttemptState = "failed"
	CanceledPaymentAttemptState               PaymentAttemptState = "canceled"
	ReconciliationRequiredPaymentAttemptState PaymentAttemptState = "reconciliation_required"
	ManualReviewRequiredPaymentAttemptState   PaymentAttemptState = "manual_review_required"
	// ExpiredPaymentAttemptState means BNPL's local CreateCheckoutPayment window
	// closed. It is not an authoritative Financial Core failure, cancellation,
	// or proof that no financial operation occurred. financial_outcome_resolved
	// stays false until a verified late FC outcome is applied. Local session
	// creation remains closed; late authoritative results may still transition
	// out of expired.
	ExpiredPaymentAttemptState PaymentAttemptState = "expired"
)

var paymentAttemptStateTransitions = newStatusTransitionTable(
	"payment attempt state",
	map[PaymentAttemptState][]PaymentAttemptState{
		CreatedPaymentAttemptState: {
			SubmittedPaymentAttemptState,
			SucceededPaymentAttemptState,
			FailedPaymentAttemptState,
			CanceledPaymentAttemptState,
			ReconciliationRequiredPaymentAttemptState,
			ExpiredPaymentAttemptState,
		},
		SubmittedPaymentAttemptState: {
			SucceededPaymentAttemptState,
			FailedPaymentAttemptState,
			CanceledPaymentAttemptState,
			ReconciliationRequiredPaymentAttemptState,
		},
		ManualReviewRequiredPaymentAttemptState: {SucceededPaymentAttemptState, FailedPaymentAttemptState, CanceledPaymentAttemptState},
		ReconciliationRequiredPaymentAttemptState: {
			SubmittedPaymentAttemptState,
			SucceededPaymentAttemptState,
			FailedPaymentAttemptState,
			CanceledPaymentAttemptState,
			ManualReviewRequiredPaymentAttemptState,
			ExpiredPaymentAttemptState,
		},
		// Expired is locally closed for CreateCheckoutPayment but must still
		// accept a recovered usable session or an authoritative late outcome.
		ExpiredPaymentAttemptState: {
			SubmittedPaymentAttemptState,
			SucceededPaymentAttemptState,
			FailedPaymentAttemptState,
			CanceledPaymentAttemptState,
		},
	},
	[]PaymentAttemptState{
		SucceededPaymentAttemptState,
		FailedPaymentAttemptState,
		CanceledPaymentAttemptState,
	},
)

// IsValid reports whether the payment attempt state is known by the domain.
func (s PaymentAttemptState) IsValid() bool {
	return paymentAttemptStateTransitions.isValid(s)
}

// CanTransitionTo reports whether the payment attempt state may change to next.
func (s PaymentAttemptState) CanTransitionTo(next PaymentAttemptState) bool {
	return paymentAttemptStateTransitions.canTransitionTo(s, next)
}

// ValidateTransitionTo returns an error when the payment attempt state may not change to next.
func (s PaymentAttemptState) ValidateTransitionTo(next PaymentAttemptState) error {
	return paymentAttemptStateTransitions.validateTransitionTo(s, next)
}

// IsTerminal reports whether the payment attempt state is terminal.
func (s PaymentAttemptState) IsTerminal() bool {
	return paymentAttemptStateTransitions.isTerminal(s)
}

// PaymentChannel identifies how the customer initiated repayment.
type PaymentChannel string

// Known repayment initiation channels.
const (
	GatewayPaymentChannel      PaymentChannel = "gateway"
	BankTransferPaymentChannel PaymentChannel = "bank_transfer"
	WalletPaymentChannel       PaymentChannel = "wallet"
)

// IsValid reports whether the payment channel is supported.
func (c PaymentChannel) IsValid() bool {
	switch c {
	case GatewayPaymentChannel, BankTransferPaymentChannel, WalletPaymentChannel:
		return true

	default:
		return false
	}
}

// PaymentReconciliationReason identifies why a Financial Core payment could not be
// applied automatically to a BNPL obligation.
type PaymentReconciliationReason string

// Known reasons a Financial Core payment could not be applied automatically.
const (
	ContractExpiredPaymentWasOnTimePaymentReconciliationReason          PaymentReconciliationReason = "contract_expired_payment_was_on_time"
	PaymentAfterDeadlinePaymentReconciliationReason                     PaymentReconciliationReason = "payment_after_deadline"
	PaymentAmountMismatchPaymentReconciliationReason                    PaymentReconciliationReason = "payment_amount_mismatch"
	PaymentCurrencyMismatchPaymentReconciliationReason                  PaymentReconciliationReason = "payment_currency_mismatch"
	InstallmentAlreadyPaidByOtherPaymentPaymentReconciliationReason     PaymentReconciliationReason = "installment_already_paid_by_other_payment"
	PaymentReferenceMismatchPaymentReconciliationReason                 PaymentReconciliationReason = "payment_reference_mismatch"
	InvalidContractStatePaymentReconciliationReason                     PaymentReconciliationReason = "invalid_contract_state"
	InvalidInstallmentStatePaymentReconciliationReason                  PaymentReconciliationReason = "invalid_installment_state"
	InvalidPaymentAttemptStatePaymentReconciliationReason               PaymentReconciliationReason = "invalid_payment_attempt_state"
	DuplicateSuccessfulPaymentPaymentReconciliationReason               PaymentReconciliationReason = "duplicate_successful_payment"
	ExposureInconsistencyPaymentReconciliationReason                    PaymentReconciliationReason = "exposure_inconsistency"
	UnknownPaymentAttemptPaymentReconciliationReason                    PaymentReconciliationReason = "unknown_payment_attempt"
	UnsupportedPaymentPurposePaymentReconciliationReason                PaymentReconciliationReason = "unsupported_payment_purpose"
	EventIdentityPayloadMismatchPaymentReconciliationReason             PaymentReconciliationReason = "event_identity_payload_mismatch"
	PaymentAttemptSnapshotInconsistentPaymentReconciliationReason       PaymentReconciliationReason = "payment_attempt_snapshot_inconsistent"
	ContractNotFoundPaymentReconciliationReason                         PaymentReconciliationReason = "contract_not_found"
	InstallmentNotFoundPaymentReconciliationReason                      PaymentReconciliationReason = "installment_not_found"
	FirstInstallmentChargeStateInconsistentPaymentReconciliationReason  PaymentReconciliationReason = "first_installment_charge_state_inconsistent"
	FirstInstallmentChargePolicyInconsistentPaymentReconciliationReason PaymentReconciliationReason = "first_installment_charge_policy_inconsistent"
	InsufficientCreditAtActivationPaymentReconciliationReason           PaymentReconciliationReason = "insufficient_credit_at_activation"
	ExposureReservationInconsistentPaymentReconciliationReason          PaymentReconciliationReason = "exposure_reservation_inconsistent"
	PaymentSessionCreationTimeoutPaymentReconciliationReason            PaymentReconciliationReason = "payment_session_creation_timeout"
	PaymentResultTimeoutPaymentReconciliationReason                     PaymentReconciliationReason = "payment_result_timeout"
	FinancialCoreStatusStillUnknownPaymentReconciliationReason          PaymentReconciliationReason = "financial_core_status_still_unknown"
)

// IsValid reports whether the reconciliation reason is known by the domain.
func (r PaymentReconciliationReason) IsValid() bool {
	switch r {
	case ContractExpiredPaymentWasOnTimePaymentReconciliationReason,
		PaymentAfterDeadlinePaymentReconciliationReason,
		PaymentAmountMismatchPaymentReconciliationReason,
		PaymentCurrencyMismatchPaymentReconciliationReason,
		InstallmentAlreadyPaidByOtherPaymentPaymentReconciliationReason,
		PaymentReferenceMismatchPaymentReconciliationReason,
		InvalidContractStatePaymentReconciliationReason,
		InvalidInstallmentStatePaymentReconciliationReason,
		InvalidPaymentAttemptStatePaymentReconciliationReason,
		DuplicateSuccessfulPaymentPaymentReconciliationReason,
		ExposureInconsistencyPaymentReconciliationReason,
		UnknownPaymentAttemptPaymentReconciliationReason,
		UnsupportedPaymentPurposePaymentReconciliationReason,
		EventIdentityPayloadMismatchPaymentReconciliationReason,
		PaymentAttemptSnapshotInconsistentPaymentReconciliationReason,
		ContractNotFoundPaymentReconciliationReason,
		InstallmentNotFoundPaymentReconciliationReason,
		FirstInstallmentChargeStateInconsistentPaymentReconciliationReason,
		FirstInstallmentChargePolicyInconsistentPaymentReconciliationReason,
		InsufficientCreditAtActivationPaymentReconciliationReason,
		ExposureReservationInconsistentPaymentReconciliationReason,
		PaymentSessionCreationTimeoutPaymentReconciliationReason,
		PaymentResultTimeoutPaymentReconciliationReason,
		FinancialCoreStatusStillUnknownPaymentReconciliationReason:
		return true

	default:
		return false
	}
}

// AllowsRecoveredPaymentSession reports whether this reconciliation reason is
// unresolved session-creation uncertainty. A valid Financial Core session for
// the same operation may still be stored. Identity-conflict reasons must not.
func (r PaymentReconciliationReason) AllowsRecoveredPaymentSession() bool {
	switch r {
	case PaymentSessionCreationTimeoutPaymentReconciliationReason,
		FinancialCoreStatusStillUnknownPaymentReconciliationReason:
		return true

	default:
		return false
	}
}

// CanReserveFinancialSessionCreate reports whether a customer request may
// start another synchronous CreateCheckoutPayment for this attempt.
//
// Installment repayment uses only created: after local cancel/expire or once
// submitted, CreateCheckoutPayment is never reopened on the same attempt.
// Checkout-first-installment may still reserve while reconciliation_required
// for recoverable session-creation uncertainty.
func (a PaymentAttempt) CanReserveFinancialSessionCreate() bool {
	if a.Purpose == InstallmentRepaymentFinancialCorePaymentPurpose {
		return a.State == CreatedPaymentAttemptState
	}

	switch a.State {
	case CreatedPaymentAttemptState:
		return true

	case ReconciliationRequiredPaymentAttemptState:
		return a.ReconciliationReason != nil && a.ReconciliationReason.AllowsRecoveredPaymentSession()

	default:
		return false
	}
}

// hasCustomerActionablePaymentURL reports whether a PaymentURL was persisted
// for customer redirect. Partial FC identity without a URL is not actionable.
func (a PaymentAttempt) hasCustomerActionablePaymentURL() bool {
	return a.PaymentURL != nil && strings.TrimSpace(*a.PaymentURL) != ""
}

// IsLocalSessionCreationRetryExhausted reports the scoped local cancellation
// after three failed CreateCheckoutPayment invocations. financial_outcome_resolved
// stays false; this is not authoritative FC cancellation.
func (a PaymentAttempt) IsLocalSessionCreationRetryExhausted() bool {
	if a.Purpose != InstallmentRepaymentFinancialCorePaymentPurpose {
		return false
	}

	if a.State != CanceledPaymentAttemptState || a.FinancialOutcomeResolved {
		return false
	}

	if a.FailureCode == nil || strings.TrimSpace(*a.FailureCode) != SessionCreationRetryExhaustedFailureCode {
		return false
	}

	if a.SubmittedAt != nil {
		return false
	}

	return !a.hasCustomerActionablePaymentURL()
}

// IsAbandonedSessionCreationExpiration reports a locally expired installment
// repayment creation that never reached submitted and never exposed a payment
// link. Distinct from ambiguous legacy expired rows that carried a session.
func (a PaymentAttempt) IsAbandonedSessionCreationExpiration() bool {
	if a.Purpose != InstallmentRepaymentFinancialCorePaymentPurpose {
		return false
	}

	if a.State != ExpiredPaymentAttemptState || a.FinancialOutcomeResolved {
		return false
	}

	if a.SubmittedAt != nil {
		return false
	}

	if a.hasCustomerActionablePaymentURL() {
		return false
	}

	if a.FinancialPaymentSessionID != nil && strings.TrimSpace(*a.FinancialPaymentSessionID) != "" {
		return false
	}

	return true
}

// BlocksNewPaymentAttempt reports whether this attempt still prevents creating
// another PaymentAttempt for the same installment. Locally canceled retry
// exhaustion and abandoned session-creation expiration deliberately do not
// block a new customer attempt with a new idempotency key.
func (a PaymentAttempt) BlocksNewPaymentAttempt() bool {
	if a.IsLocalSessionCreationRetryExhausted() || a.IsAbandonedSessionCreationExpiration() {
		return false
	}

	switch a.State {
	case CreatedPaymentAttemptState,
		SubmittedPaymentAttemptState,
		ReconciliationRequiredPaymentAttemptState,
		ManualReviewRequiredPaymentAttemptState:
		return true

	case FailedPaymentAttemptState, CanceledPaymentAttemptState, ExpiredPaymentAttemptState:
		return !a.FinancialOutcomeResolved

	default:
		return false
	}
}

// SuppressesAutomaticSessionCreationReconciliation reports whether BNPL must
// stop enqueueing automatic Financial Core session-creation rediscovery for
// residual historical reconciliation_required rows that never received a
// customer-actionable session. New installment repayment flows cancel after
// three sync failures or expire abandoned creation instead of rediscovery.
// Submitted-payment outcome reconciliation (payment_result_timeout) is unaffected.
func (a PaymentAttempt) SuppressesAutomaticSessionCreationReconciliation() bool {
	if a.Purpose != InstallmentRepaymentFinancialCorePaymentPurpose {
		return false
	}

	if a.State != ReconciliationRequiredPaymentAttemptState {
		return false
	}

	if a.ReconciliationReason == nil || !a.ReconciliationReason.AllowsRecoveredPaymentSession() {
		return false
	}

	// A partial session ID does not justify initiating automatic recovery
	// for a historical session-creation uncertainty. Reconciliation of an
	// actually submitted payment is controlled by a different reason.
	return true
}

// EligibleForSessionCreationExpiration reports whether this installment
// repayment attempt may be transitioned to expired by the local cleanup job.
// safetyAfterLease is added after FinancialSessionCreateLeaseUntil so an
// in-flight third CreateCheckoutPayment is not invalidated at TTL boundaries.
func (a PaymentAttempt) EligibleForSessionCreationExpiration(now time.Time, safetyAfterLease time.Duration) bool {
	if a.Purpose != InstallmentRepaymentFinancialCorePaymentPurpose {
		return false
	}

	if a.FinancialOutcomeResolved {
		return false
	}

	if a.FinancialPaymentSessionID != nil && strings.TrimSpace(*a.FinancialPaymentSessionID) != "" {
		return false
	}

	switch a.State {
	case CreatedPaymentAttemptState:
		// ok

	case ReconciliationRequiredPaymentAttemptState:
		if a.ReconciliationReason == nil || !a.ReconciliationReason.AllowsRecoveredPaymentSession() {
			return false
		}

	default:
		return false
	}

	if now.Before(a.CreatedAt.Add(InstallmentRepaymentPaymentSessionTTL)) {
		return false
	}

	if a.FinancialSessionCreateLeaseActive(now) {
		return false
	}

	if a.FinancialSessionCreateLeaseUntil != nil {
		safeAfter := a.FinancialSessionCreateLeaseUntil.Add(safetyAfterLease)
		if now.Before(safeAfter) {
			return false
		}
	}

	return true
}

// FinancialSessionCreateLeaseActive reports whether another caller already
// owns the right to start CreateCheckoutPayment. Expiry is not payment failure.
func (a PaymentAttempt) FinancialSessionCreateLeaseActive(now time.Time) bool {
	return a.FinancialSessionCreateLeaseUntil != nil && a.FinancialSessionCreateLeaseUntil.After(now)
}

// FinancialCorePaymentPurpose identifies why Financial Core processed a payment.
type FinancialCorePaymentPurpose string

// Known Financial Core payment purposes handled by BNPL.
const (
	CheckoutFirstInstallmentFinancialCorePaymentPurpose FinancialCorePaymentPurpose = "checkout_first_installment"
	InstallmentRepaymentFinancialCorePaymentPurpose     FinancialCorePaymentPurpose = "installment_repayment"
)

// IsValid reports whether the payment purpose is known by the domain.
func (p FinancialCorePaymentPurpose) IsValid() bool {
	switch p {
	case CheckoutFirstInstallmentFinancialCorePaymentPurpose,
		InstallmentRepaymentFinancialCorePaymentPurpose:
		return true

	default:
		return false
	}
}

// FinancialPaymentResultStatus is the typed BNPL interpretation of a Financial Core payment result.
type FinancialPaymentResultStatus string

// Known Financial Core payment result statuses handled by BNPL.
const (
	SucceededFinancialPaymentResultStatus FinancialPaymentResultStatus = "succeeded"
	FailedFinancialPaymentResultStatus    FinancialPaymentResultStatus = "failed"
	CanceledFinancialPaymentResultStatus  FinancialPaymentResultStatus = "canceled"
)

// IsValid reports whether the payment result status is supported by BNPL handlers.
func (s FinancialPaymentResultStatus) IsValid() bool {
	switch s {
	case SucceededFinancialPaymentResultStatus,
		FailedFinancialPaymentResultStatus,
		CanceledFinancialPaymentResultStatus:
		return true

	default:
		return false
	}
}

// PaymentAttempt stores a local BNPL repayment orchestration record.
type PaymentAttempt struct {
	// FinancialOutcomeResolved records authoritative terminal FC outcome, not local expiry/cancel.
	FinancialOutcomeResolved bool `db:"financial_outcome_resolved" json:"-"`

	// ID uniquely identifies this payment attempt.
	ID PaymentAttemptID `db:"id" json:"id,omitempty"`

	// ContractID references the contract being repaid.
	ContractID ContractID `db:"contract_id" json:"contract_id,omitempty"`

	// InstallmentID references the installment targeted by this payment attempt.
	InstallmentID InstallmentID `db:"installment_id" json:"installment_id,omitempty"`

	// AmountMinor is the total payable amount for this attempt in minor units.
	AmountMinor uint64 `db:"amount_minor" json:"amount_minor,omitempty"`

	// Currency is the currency of AmountMinor.
	Currency Currency `db:"currency" json:"currency,omitempty"`

	// Purpose identifies why Financial Core should process this payment.
	Purpose FinancialCorePaymentPurpose `db:"purpose" json:"purpose,omitempty"`

	// TenantID optionally identifies the Financial Core tenant for this attempt.
	TenantID *string `db:"tenant_id" json:"tenant_id,omitempty"`

	// ContextID optionally identifies the Financial Core context for this attempt.
	ContextID *string `db:"context_id" json:"context_id,omitempty"`

	// ProviderCode optionally identifies the payment provider selected for checkout.
	ProviderCode *string `db:"provider_code" json:"provider_code,omitempty"`

	// SuccessReturnURL is the browser return URL used after a successful payment.
	SuccessReturnURL *string `db:"success_return_url" json:"success_return_url,omitempty"`

	// FailureReturnURL is the browser return URL used after a failed payment.
	FailureReturnURL *string `db:"failure_return_url" json:"failure_return_url,omitempty"`

	// CancelReturnURL is the browser return URL used after a canceled payment.
	CancelReturnURL *string `db:"cancel_return_url" json:"cancel_return_url,omitempty"`

	// State is the local BNPL repayment orchestration state.
	State PaymentAttemptState `db:"state" json:"state,omitempty"`

	// Channel identifies how the customer initiated repayment.
	Channel PaymentChannel `db:"channel" json:"channel,omitempty"`

	// IdempotencyKey is the client/use-case idempotency key for this attempt.
	IdempotencyKey string `db:"idempotency_key" json:"-"`

	// RequestFingerprint fingerprints the create-payment request for reuse detection.
	RequestFingerprint string `db:"request_fingerprint" json:"-"`

	// FinancialCollectionObligationID is the Financial Core collection obligation prepared before checkout.
	FinancialCollectionObligationID *string `db:"financial_collection_obligation_id" json:"financial_collection_obligation_id,omitempty"`

	// FinancialPaymentIntentID is the Financial Core payment-intent id for this attempt.
	FinancialPaymentIntentID *string `db:"financial_payment_intent_id" json:"financial_payment_intent_id,omitempty"`

	// FinancialPaymentAttemptID is the Financial Core payment-attempt id snapshot.
	FinancialPaymentAttemptID *string `db:"financial_payment_attempt_id" json:"financial_payment_attempt_id,omitempty"`

	// FinancialPaymentSessionID is the Financial Core payment-session id snapshot.
	FinancialPaymentSessionID *string `db:"financial_payment_session_id" json:"financial_payment_session_id,omitempty"`

	// PaymentURL is the customer-facing redirect URL for completing payment.
	PaymentURL *string `db:"payment_url" json:"payment_url,omitempty"`

	// FinancialPaymentExpiresAt is when the Financial Core payment session expires.
	FinancialPaymentExpiresAt *time.Time `db:"financial_payment_expires_at" json:"expires_at,omitempty"`

	// FailureCode classifies why the payment attempt failed, if applicable.
	FailureCode *string `db:"failure_code" json:"failure_code,omitempty"`

	// FinancialSessionCreateAttemptCount counts sync CreateCheckoutPayment calls
	// for this immutable attempt. Capped by MaxFinancialSessionCreateAttempts.
	FinancialSessionCreateAttemptCount int `db:"financial_session_create_attempt_count" json:"financial_session_create_attempt_count,omitempty"`

	// FinancialSessionCreateLeaseUntil is when the current in-flight
	// CreateCheckoutPayment reservation expires. A future value blocks another
	// reservation. Expiry does not prove Financial Core did not run the call.
	FinancialSessionCreateLeaseUntil *time.Time `db:"financial_session_create_lease_until" json:"-"`

	// FinancialSessionCreateLeaseToken identifies the reservation that may
	// start the external call. It does not invalidate a verified Financial
	// Core session that arrives after the lease expires.
	FinancialSessionCreateLeaseToken *uuid.UUID `db:"financial_session_create_lease_token" json:"-"`

	// ReconciliationReason identifies why automatic application could not complete.
	ReconciliationReason *PaymentReconciliationReason `db:"reconciliation_reason" json:"reconciliation_reason,omitempty"`

	// ReconciliationAttemptCount is how many reconciliation attempts have been made.
	ReconciliationAttemptCount int `db:"reconciliation_attempt_count" json:"reconciliation_attempt_count,omitempty"`

	// ReconciliationRequestedAt is when reconciliation was first requested.
	ReconciliationRequestedAt *time.Time `db:"reconciliation_requested_at" json:"reconciliation_requested_at,omitempty"`

	// NextReconciliationAt is the earliest time another reconciliation attempt may run.
	NextReconciliationAt *time.Time `db:"next_reconciliation_at" json:"next_reconciliation_at,omitempty"`

	// ReconciliationResolvedAt is when reconciliation completed successfully.
	ReconciliationResolvedAt *time.Time `db:"reconciliation_resolved_at" json:"reconciliation_resolved_at,omitempty"`

	// LastReconciliationError stores the most recent reconciliation failure detail.
	LastReconciliationError *string `db:"last_reconciliation_error" json:"last_reconciliation_error,omitempty"`

	// ManualReviewRequiredAt is when the attempt was flagged for manual review.
	ManualReviewRequiredAt *time.Time `db:"manual_review_required_at" json:"manual_review_required_at,omitempty"`

	// SubmittedAt is when the payment was submitted to Financial Core.
	SubmittedAt *time.Time `db:"submitted_at" json:"submitted_at,omitempty"`

	// FinancialPaidAt is when Financial Core reported the payment as paid.
	FinancialPaidAt *time.Time `db:"financial_paid_at" json:"financial_paid_at,omitempty"`

	// AppliedAt is when the paid result was applied to local BNPL state.
	AppliedAt *time.Time `db:"applied_at" json:"applied_at,omitempty"`

	// FailedAt is when the payment attempt permanently failed.
	FailedAt *time.Time `db:"failed_at" json:"failed_at,omitempty"`

	// CanceledAt is when the payment attempt was canceled.
	CanceledAt *time.Time `db:"canceled_at" json:"canceled_at,omitempty"`

	// ExpiredAt is when BNPL closed the local CreateCheckoutPayment window.
	// It is not an authoritative Financial Core payment outcome timestamp.
	ExpiredAt *time.Time `db:"expired_at" json:"expired_at,omitempty"`

	// CreatedAt is when the payment attempt was created.
	CreatedAt time.Time `db:"created_at" json:"created_at,omitempty"`

	// UpdatedAt is when the payment attempt was last modified.
	UpdatedAt time.Time `db:"updated_at" json:"updated_at,omitempty"`

	// Items holds snapshotted payable obligation lines when the aggregate is loaded with items.
	Items []PaymentAttemptItem `db:"-" json:"items,omitempty"`
}
