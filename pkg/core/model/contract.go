package model

import (
	"database/sql/driver"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ContractID uniquely identifies a Contract.
type ContractID uuid.UUID

// ParseContractID parses a ContractID from its canonical string form.
func ParseContractID(value string) (ContractID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return ContractID{}, fmt.Errorf("parse contract id: %w", err)
	}

	return ContractID(id), nil
}

func (id ContractID) UUID() uuid.UUID { return uuid.UUID(id) }

func (id ContractID) String() string { return uuid.UUID(id).String() }

func (id ContractID) IsZero() bool { return uuid.UUID(id) == uuid.Nil }

func (id ContractID) MarshalText() ([]byte, error) { return uuid.UUID(id).MarshalText() }

func (id *ContractID) UnmarshalText(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data); err != nil {
		return err
	}

	*id = ContractID(u)

	return nil
}

func (id *ContractID) Scan(src any) error {
	var u uuid.UUID
	if err := u.Scan(src); err != nil {
		return err
	}

	*id = ContractID(u)

	return nil
}

func (id ContractID) Value() (driver.Value, error) { return uuid.UUID(id).Value() }

// ContractStatus identifies the lifecycle state of a BNPL contract.
type ContractStatus string

// Known BNPL contract lifecycle statuses.
const (
	PaymentPendingContractStatus ContractStatus = "payment_pending"
	ActiveContractStatus         ContractStatus = "active"
	CancelingContractStatus      ContractStatus = "canceling"
	CompletedContractStatus      ContractStatus = "completed"
	CanceledContractStatus       ContractStatus = "canceled"
	ExpiredContractStatus        ContractStatus = "expired"
	DefaultedContractStatus      ContractStatus = "defaulted"
)

var contractStatusTransitions = newStatusTransitionTable(
	"contract status",
	map[ContractStatus][]ContractStatus{
		PaymentPendingContractStatus: {
			ActiveContractStatus,
			CanceledContractStatus,
			ExpiredContractStatus,
		},
		ActiveContractStatus: {
			CompletedContractStatus,
			CancelingContractStatus,
			DefaultedContractStatus,
		},
		// canceling → canceled is reserved for authoritative cancellation
		// completion. canceling → active is the compensating transition used
		// only when the cancellation request is definitively rejected before
		// any authoritative cancellation effect is accepted.
		CancelingContractStatus: {
			ActiveContractStatus,
			CanceledContractStatus,
		},
	},
	[]ContractStatus{
		CompletedContractStatus,
		CanceledContractStatus,
		ExpiredContractStatus,
		DefaultedContractStatus,
	},
)

// IsValid reports whether the contract status is known by the domain.
func (s ContractStatus) IsValid() bool {
	return contractStatusTransitions.isValid(s)
}

// CanTransitionTo reports whether the contract status may change to next.
func (s ContractStatus) CanTransitionTo(next ContractStatus) bool {
	return contractStatusTransitions.canTransitionTo(s, next)
}

// ValidateTransitionTo returns an error when the contract status may not change to next.
func (s ContractStatus) ValidateTransitionTo(next ContractStatus) error {
	return contractStatusTransitions.validateTransitionTo(s, next)
}

// IsTerminal reports whether the contract status is terminal.
func (s ContractStatus) IsTerminal() bool {
	return contractStatusTransitions.isTerminal(s)
}

// IsPaymentPending reports whether the contract is in the pre-activation
// financial-fulfillment state: it is waiting for Financial Core success before
// it may become active. That success is either the customer's initial payment
// or, for merchant-funding policies, FC merchant funding / economic
// recognition. It does NOT imply that a PaymentAttempt exists.
func (s ContractStatus) IsPaymentPending() bool {
	return s == PaymentPendingContractStatus
}

// IsActive reports whether the contract is financially active.
func (s ContractStatus) IsActive() bool {
	return s == ActiveContractStatus
}

// IsCanceling reports whether a cancellation process is in progress. The local
// state may precede external acceptance and remains non-terminal until an
// authoritative cancellation result completes or the process is compensated.
func (s ContractStatus) IsCanceling() bool {
	return s == CancelingContractStatus
}

// IsCanceled reports whether the contract is terminally canceled.
func (s ContractStatus) IsCanceled() bool {
	return s == CanceledContractStatus
}

// ContractCancellationReason identifies why a customer-initiated contract
// cancellation was requested.
//
// Reasons are canonical wire values. Add new constants here when additional
// cancellation reasons become supported; do not scatter magic strings.
type ContractCancellationReason string

const (
	// CustomerRequestedContractCancellationReason means the authenticated
	// customer explicitly requested cancellation of an active contract.
	CustomerRequestedContractCancellationReason ContractCancellationReason = "customer_requested"
)

// IsValid reports whether the cancellation reason is known by the domain.
func (r ContractCancellationReason) IsValid() bool {
	switch r {
	case CustomerRequestedContractCancellationReason:
		return true

	default:
		return false
	}
}

// String returns the canonical wire representation of the cancellation reason.
func (r ContractCancellationReason) String() string {
	return string(r)
}

// Contract represents a checkout BNPL contract across its full lifecycle.
type Contract struct {
	// ID uniquely identifies this BNPL contract.
	ID ContractID `db:"id" json:"id"`

	// DecisionID references the checkout decision that created this contract.
	DecisionID DecisionID `db:"decision_id" json:"decision_id"`

	// CheckoutKey is the merchant checkout identity associated with this contract.
	CheckoutKey string `db:"checkout_key" json:"checkout_key"`

	// CustomerID is the internal customer profile primary key for this contract.
	CustomerID CustomerID `db:"customer_id" json:"customer_id"`

	// CreditAccountID identifies the credit-capacity account whose exposure this contract consumes.
	CreditAccountID CreditAccountID `db:"credit_account_id" json:"credit_account_id"`

	// MerchantProfileID identifies the merchant BNPL profile for this contract.
	MerchantProfileID MerchantProfileID `db:"merchant_profile_id" json:"merchant_profile_id"`

	// MerchantBranchProfileID optionally identifies the branch profile used for checkout.
	MerchantBranchProfileID *MerchantBranchProfileID `db:"merchant_branch_profile_id" json:"merchant_branch_profile_id,omitempty"`

	// Currency is the currency of all monetary fields on this contract.
	Currency Currency `db:"currency" json:"currency"`

	// RequestedAmountMinor is the original checkout request amount in minor units.
	RequestedAmountMinor uint64 `db:"requested_amount_minor" json:"requested_amount_minor"`

	// ApprovedFinancedAmountMinor is the financed amount approved for the customer.
	ApprovedFinancedAmountMinor uint64 `db:"approved_financed_amount_minor" json:"approved_financed_amount_minor"`

	// UpfrontAmountMinor is the amount the customer must pay up front.
	UpfrontAmountMinor uint64 `db:"upfront_amount_minor" json:"upfront_amount_minor"`

	// InstallmentCount is the number of installments in the contract schedule.
	InstallmentCount int `db:"installment_count" json:"installment_count"`

	// PayableNowAmountMinor is the amount currently due for immediate payment.
	PayableNowAmountMinor uint64 `db:"payable_now_amount_minor" json:"payable_now_amount_minor"`

	// RemainingPrincipalAmountMinor is the outstanding financed principal in minor units.
	RemainingPrincipalAmountMinor uint64 `db:"remaining_principal_amount_minor" json:"remaining_principal_amount_minor"`

	// Status is the current lifecycle state of the contract.
	Status ContractStatus `db:"status" json:"status"`

	// PaymentReference is an optional external payment reference for the contract.
	PaymentReference *string `db:"payment_reference" json:"payment_reference,omitempty"`

	// PaymentID optionally references a local payment orchestration record.
	PaymentID *PaymentAttemptID `db:"payment_id" json:"payment_id,omitempty"`

	// FinancialInvoiceID is the Financial Core invoice id snapshot for checkout payment.
	FinancialInvoiceID *string `db:"financial_invoice_id" json:"financial_invoice_id,omitempty"`

	// FinancialMerchantEntitlementID is the Financial Core merchant entitlement recorded after activation.
	FinancialMerchantEntitlementID *string `db:"financial_merchant_entitlement_id" json:"financial_merchant_entitlement_id,omitempty"`

	// FinancialReceivableID is the Financial Core receivable created for the financed principal.
	FinancialReceivableID *string `db:"financial_receivable_id" json:"financial_receivable_id,omitempty"`

	// FinancialPaymentIntentID is the Financial Core payment-intent id snapshot.
	FinancialPaymentIntentID *string `db:"financial_payment_intent_id" json:"financial_payment_intent_id,omitempty"`

	// FinancialPaymentAttemptID is the latest Financial Core payment-attempt id snapshot.
	FinancialPaymentAttemptID *string `db:"financial_payment_attempt_id" json:"financial_payment_attempt_id,omitempty"`

	// SuccessfulFinancialPaymentAttemptID is the Financial Core attempt id that succeeded.
	SuccessfulFinancialPaymentAttemptID *string `db:"successful_financial_payment_attempt_id" json:"successful_financial_payment_attempt_id,omitempty"`

	// FinancialPaymentSessionID is the Financial Core payment-session id snapshot.
	FinancialPaymentSessionID *string `db:"financial_payment_session_id" json:"financial_payment_session_id,omitempty"`

	// FinancialPaymentLink is the customer-facing Financial Core payment URL.
	FinancialPaymentLink *string `db:"financial_payment_link" json:"financial_payment_link,omitempty"`

	// FinancialPaymentExpiresAt is when the Financial Core payment session expires.
	FinancialPaymentExpiresAt *time.Time `db:"financial_payment_expires_at" json:"financial_payment_expires_at,omitempty"`

	// FinancialPaymentReusedIdempotentResult reports whether checkout reused an idempotent Financial Core result.
	FinancialPaymentReusedIdempotentResult bool `db:"financial_payment_reused_idempotent_result" json:"financial_payment_reused_idempotent_result"`

	// ExpiresAt is when the pending contract expires if not activated.
	ExpiresAt time.Time `db:"expires_at" json:"expires_at"`

	// ActivatedAt is when the contract became active after its policy-required Financial Core fulfillment completed authoritatively.
	ActivatedAt *time.Time `db:"activated_at" json:"activated_at,omitempty"`

	// CompletedAt is when the contract was fully repaid and closed.
	CompletedAt *time.Time `db:"completed_at" json:"completed_at,omitempty"`

	// CanceledAt is when the contract was canceled.
	CanceledAt *time.Time `db:"canceled_at" json:"canceled_at,omitempty"`

	// CancellationIdempotencyKey is the client idempotency key for contract cancellation.
	CancellationIdempotencyKey *string `db:"cancellation_idempotency_key" json:"cancellation_idempotency_key,omitempty"`

	// CancellationReason identifies why cancellation was requested.
	CancellationReason *ContractCancellationReason `db:"cancellation_reason" json:"cancellation_reason,omitempty"`

	// CancellationRequestFingerprint fingerprints the cancellation request for reuse detection.
	CancellationRequestFingerprint *string `db:"cancellation_request_fingerprint" json:"cancellation_request_fingerprint,omitempty"`

	// CancellationRequestedAt is when cancellation was requested.
	CancellationRequestedAt *time.Time `db:"cancellation_requested_at" json:"cancellation_requested_at,omitempty"`

	// CancellationFinancialCoreIdempotencyKey is the Financial Core idempotency key for cancel.
	CancellationFinancialCoreIdempotencyKey *string `db:"cancellation_financial_core_idempotency_key" json:"cancellation_financial_core_idempotency_key,omitempty"`

	// CancellationTenantID is the tenant correlation id retained for cancellation Saga work.
	CancellationTenantID *TenantID `db:"cancellation_tenant_id" json:"-"`

	// CancellationContextID is the context correlation id retained for cancellation Saga work.
	CancellationContextID *ContextID `db:"cancellation_context_id" json:"-"`

	// CancellationFinancialCoreAcceptedAt is when Financial Core accepted the cancel request.
	CancellationFinancialCoreAcceptedAt *time.Time `db:"cancellation_financial_core_accepted_at" json:"-"`

	// CreatedAt is when the contract was created.
	CreatedAt time.Time `db:"created_at" json:"created_at"`

	// UpdatedAt is when the contract was last modified.
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}
