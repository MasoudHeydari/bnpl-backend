package model

import (
	"database/sql/driver"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// CreditAccountID uniquely identifies a customer's BNPL credit account.
type CreditAccountID uuid.UUID

// ParseCreditAccountID parses a CreditAccountID from its canonical string form.
func ParseCreditAccountID(value string) (CreditAccountID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return CreditAccountID{}, fmt.Errorf("parse credit account id: %w", err)
	}

	return CreditAccountID(id), nil
}

func (id CreditAccountID) UUID() uuid.UUID              { return uuid.UUID(id) }
func (id CreditAccountID) String() string               { return uuid.UUID(id).String() }
func (id CreditAccountID) IsZero() bool                 { return uuid.UUID(id) == uuid.Nil }
func (id CreditAccountID) MarshalText() ([]byte, error) { return uuid.UUID(id).MarshalText() }

func (id *CreditAccountID) UnmarshalText(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data); err != nil {
		return err
	}

	*id = CreditAccountID(u)
	return nil
}

func (id *CreditAccountID) Scan(src any) error {
	var u uuid.UUID
	if err := u.Scan(src); err != nil {
		return err
	}

	*id = CreditAccountID(u)
	return nil
}

func (id CreditAccountID) Value() (driver.Value, error) { return uuid.UUID(id).Value() }

// CreditAccountStatus identifies whether the current credit capacity may be used
// for new BNPL commitments. Exposure can remain positive after an account is
// expired or revoked; those states block new commitments but never rewrite an
// already-existing obligation.
type CreditAccountStatus string

const (
	ActiveCreditAccountStatus  CreditAccountStatus = "active"
	ExpiredCreditAccountStatus CreditAccountStatus = "expired"
	RevokedCreditAccountStatus CreditAccountStatus = "revoked"
)

// IsValid reports whether the credit account status is known by the domain.
func (s CreditAccountStatus) IsValid() bool {
	switch s {
	case ActiveCreditAccountStatus, ExpiredCreditAccountStatus, RevokedCreditAccountStatus:
		return true

	default:
		return false
	}
}

// IsActive reports whether the account may be used for a new credit-consuming flow.
func (s CreditAccountStatus) IsActive() bool { return s == ActiveCreditAccountStatus }

// CreditAccount is BNPL's current operational view of a customer's credit
// capacity for one currency.
//
// It is not a cash wallet and is not a financial ledger. LimitAmountMinor is
// governed BNPL capacity. ExposureAmountMinor is outstanding BNPL principal that
// consumes that capacity. Financial Core remains authoritative for payment and
// money-movement facts that cause exposure changes.
type CreditAccount struct {
	ID CreditAccountID `db:"id" json:"id"`

	CustomerID CustomerID `db:"customer_id" json:"customer_id"`
	Currency   Currency   `db:"currency" json:"currency"`

	LimitAmountMinor    uint64 `db:"limit_amount_minor" json:"limit_amount_minor"`
	ExposureAmountMinor uint64 `db:"exposure_amount_minor" json:"exposure_amount_minor"`

	Status CreditAccountStatus `db:"status" json:"status"`

	// LastLimitDecisionID is the governed decision that established the current
	// limit state. Historical limit changes are retained in credit_transactions.
	LastLimitDecisionID *DecisionID `db:"last_limit_decision_id" json:"last_limit_decision_id,omitempty"`

	EffectiveFrom time.Time  `db:"effective_from" json:"effective_from"`
	ExpiresAt     *time.Time `db:"expires_at" json:"expires_at,omitempty"`
	RevokedAt     *time.Time `db:"revoked_at" json:"revoked_at,omitempty"`

	// Version is incremented on every account mutation. Row locking remains the
	// primary concurrency mechanism; Version is retained for diagnostics and
	// future optimistic-concurrency use.
	Version int64 `db:"version" json:"version"`

	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// AvailableAmountMinor returns current non-negative available credit.
func (a CreditAccount) AvailableAmountMinor() uint64 {
	if a.LimitAmountMinor <= a.ExposureAmountMinor {
		return 0
	}

	return a.LimitAmountMinor - a.ExposureAmountMinor
}

// IsUsableAt reports whether the account is active and its current limit has
// not expired at the supplied instant.
func (a CreditAccount) IsUsableAt(at time.Time) bool {
	if !a.Status.IsActive() {
		return false
	}

	return a.ExpiresAt == nil || a.ExpiresAt.After(at)
}
