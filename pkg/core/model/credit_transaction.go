package model

import (
	"database/sql/driver"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// CreditTransactionID uniquely identifies an immutable credit-capacity journal entry.
type CreditTransactionID uuid.UUID

func (id CreditTransactionID) UUID() uuid.UUID              { return uuid.UUID(id) }
func (id CreditTransactionID) String() string               { return uuid.UUID(id).String() }
func (id CreditTransactionID) IsZero() bool                 { return uuid.UUID(id) == uuid.Nil }
func (id CreditTransactionID) Value() (driver.Value, error) { return uuid.UUID(id).Value() }
func (id *CreditTransactionID) Scan(src any) error {
	var u uuid.UUID
	if err := u.Scan(src); err != nil {
		return err
	}

	*id = CreditTransactionID(u)
	return nil
}

// CreditTransactionType describes what changed on a CreditAccount.
type CreditTransactionType string

const (
	LimitSetCreditTransactionType          CreditTransactionType = "limit_set"
	LimitIncreasedCreditTransactionType    CreditTransactionType = "limit_increased"
	LimitDecreasedCreditTransactionType    CreditTransactionType = "limit_decreased"
	ExposureIncreasedCreditTransactionType CreditTransactionType = "exposure_increased"
	ExposureDecreasedCreditTransactionType CreditTransactionType = "exposure_decreased"
)

func (t CreditTransactionType) IsValid() bool {
	switch t {
	case LimitSetCreditTransactionType,
		LimitIncreasedCreditTransactionType,
		LimitDecreasedCreditTransactionType,
		ExposureIncreasedCreditTransactionType,
		ExposureDecreasedCreditTransactionType:
		return true

	default:
		return false
	}
}

// CreditTransactionReason identifies the business fact that caused the capacity change.
type CreditTransactionReason string

const (
	CreditAssessmentCreditTransactionReason  CreditTransactionReason = "credit_assessment"
	ContractActivatedCreditTransactionReason CreditTransactionReason = "contract_activated"
	InstallmentPaidCreditTransactionReason   CreditTransactionReason = "installment_paid"
	ContractCanceledCreditTransactionReason  CreditTransactionReason = "contract_canceled"
)

func (r CreditTransactionReason) IsValid() bool {
	switch r {
	case CreditAssessmentCreditTransactionReason,
		ContractActivatedCreditTransactionReason,
		InstallmentPaidCreditTransactionReason,
		ContractCanceledCreditTransactionReason:
		return true

	default:
		return false
	}
}

// CreditTransactionReferenceType identifies the business object whose fact
// produced a credit mutation. The tuple (credit_account_id, type, reference_type, reference_id) is
// unique and is the journal's business-level idempotency boundary.
type CreditTransactionReferenceType string

const (
	DecisionCreditTransactionReferenceType    CreditTransactionReferenceType = "decision"
	ContractCreditTransactionReferenceType    CreditTransactionReferenceType = "contract"
	InstallmentCreditTransactionReferenceType CreditTransactionReferenceType = "installment"
)

func (t CreditTransactionReferenceType) IsValid() bool {
	switch t {
	case DecisionCreditTransactionReferenceType,
		ContractCreditTransactionReferenceType,
		InstallmentCreditTransactionReferenceType:
		return true

	default:
		return false
	}
}

// CreditTransaction is an immutable BNPL credit-capacity journal entry.
//
// This is deliberately not a Financial Core payment/ledger transaction. AmountMinor
// is the absolute limit for LIMIT_SET and a positive capacity/exposure delta for
// the other types; direction is defined by Type. Correlation and
// causation identifiers remain in the existing inbox/outbox/audit envelopes instead
// of being duplicated here. Idempotency is enforced by the immutable business fact
// reference rather than by an arbitrary retry key.
type CreditTransaction struct {
	ID              CreditTransactionID            `db:"id" json:"id"`
	CreditAccountID CreditAccountID                `db:"credit_account_id" json:"credit_account_id"`
	Type            CreditTransactionType          `db:"type" json:"type"`
	AmountMinor     uint64                         `db:"amount_minor" json:"amount_minor"`
	Currency        Currency                       `db:"currency" json:"currency"`
	Reason          CreditTransactionReason        `db:"reason_code" json:"reason_code"`
	ReferenceType   CreditTransactionReferenceType `db:"reference_type" json:"reference_type"`
	ReferenceID     uuid.UUID                      `db:"reference_id" json:"reference_id"`
	OccurredAt      time.Time                      `db:"occurred_at" json:"occurred_at"`
	CreatedAt       time.Time                      `db:"created_at" json:"created_at"`
}

// Validate verifies an entry before persistence.
func (t CreditTransaction) Validate() error {
	if t.CreditAccountID.IsZero() {
		return fmt.Errorf("credit account id is required")
	}

	if !t.Type.IsValid() {
		return fmt.Errorf("credit transaction type is invalid")
	}

	if t.AmountMinor == 0 {
		return fmt.Errorf("credit transaction amount must be positive")
	}

	if !t.Currency.IsValid() {
		return fmt.Errorf("credit transaction currency is invalid")
	}

	if !t.Reason.IsValid() {
		return fmt.Errorf("credit transaction reason is invalid")
	}

	if !t.ReferenceType.IsValid() {
		return fmt.Errorf("credit transaction reference type is invalid")
	}

	if t.ReferenceID == uuid.Nil {
		return fmt.Errorf("credit transaction reference id is required")
	}

	if t.OccurredAt.IsZero() || t.CreatedAt.IsZero() {
		return fmt.Errorf("credit transaction timestamps are required")
	}

	if err := t.validateBusinessFactShape(); err != nil {
		return err
	}

	return nil
}

// validateBusinessFactShape enforces the allowed type/reason/reference pairings
// so the journal cannot describe an impossible capacity-change fact.
func (t CreditTransaction) validateBusinessFactShape() error {
	switch t.Type {
	case LimitSetCreditTransactionType,
		LimitIncreasedCreditTransactionType,
		LimitDecreasedCreditTransactionType:
		if t.Reason != CreditAssessmentCreditTransactionReason {
			return fmt.Errorf("limit credit transactions require credit_assessment reason")
		}

		if t.ReferenceType != DecisionCreditTransactionReferenceType {
			return fmt.Errorf("limit credit transactions require decision reference")
		}

	case ExposureIncreasedCreditTransactionType:
		if t.Reason != ContractActivatedCreditTransactionReason {
			return fmt.Errorf("exposure_increased requires contract_activated reason")
		}

		if t.ReferenceType != ContractCreditTransactionReferenceType {
			return fmt.Errorf("exposure_increased requires contract reference")
		}

	case ExposureDecreasedCreditTransactionType:
		switch {
		case t.Reason == InstallmentPaidCreditTransactionReason &&
			t.ReferenceType == InstallmentCreditTransactionReferenceType:
			return nil

		case t.Reason == ContractCanceledCreditTransactionReason &&
			t.ReferenceType == ContractCreditTransactionReferenceType:
			return nil

		default:
			return fmt.Errorf("exposure_decreased requires installment_paid/installment or contract_canceled/contract")
		}

	default:
		return fmt.Errorf("credit transaction type is invalid")
	}

	return nil
}
