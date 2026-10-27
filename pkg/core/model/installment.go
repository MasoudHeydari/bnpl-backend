package model

import (
	"database/sql/driver"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// InstallmentID uniquely identifies an Installment.
type InstallmentID uuid.UUID

// ParseInstallmentID parses an InstallmentID from its canonical string form.
func ParseInstallmentID(value string) (InstallmentID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return InstallmentID{}, fmt.Errorf("parse installment id: %w", err)
	}

	return InstallmentID(id), nil
}

func (id InstallmentID) UUID() uuid.UUID { return uuid.UUID(id) }

func (id InstallmentID) String() string { return uuid.UUID(id).String() }

func (id InstallmentID) IsZero() bool { return uuid.UUID(id) == uuid.Nil }

func (id InstallmentID) MarshalText() ([]byte, error) { return uuid.UUID(id).MarshalText() }

func (id *InstallmentID) UnmarshalText(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data); err != nil {
		return err
	}

	*id = InstallmentID(u)

	return nil
}

func (id *InstallmentID) Scan(src any) error {
	var u uuid.UUID
	if err := u.Scan(src); err != nil {
		return err
	}

	*id = InstallmentID(u)

	return nil
}

func (id InstallmentID) Value() (driver.Value, error) { return uuid.UUID(id).Value() }

// InstallmentStatus identifies the lifecycle state of an installment offer or contract installment.
type InstallmentStatus string

// Known installment lifecycle statuses.
const (
	OfferedInstallmentStatus   InstallmentStatus = "offered"
	ScheduledInstallmentStatus InstallmentStatus = "scheduled"
	DueInstallmentStatus       InstallmentStatus = "due"
	PaidInstallmentStatus      InstallmentStatus = "paid"
	OverdueInstallmentStatus   InstallmentStatus = "overdue"
	CanceledInstallmentStatus  InstallmentStatus = "canceled"
	ExpiredInstallmentStatus   InstallmentStatus = "expired"
)

var installmentStatusTransitions = newStatusTransitionTable(
	"installment status",
	map[InstallmentStatus][]InstallmentStatus{
		OfferedInstallmentStatus: {
			ScheduledInstallmentStatus,
			DueInstallmentStatus,
			PaidInstallmentStatus,
			ExpiredInstallmentStatus,
			CanceledInstallmentStatus,
		},
		ScheduledInstallmentStatus: {
			DueInstallmentStatus,
			PaidInstallmentStatus,
			CanceledInstallmentStatus,
		},
		DueInstallmentStatus: {
			PaidInstallmentStatus,
			OverdueInstallmentStatus,
			CanceledInstallmentStatus,
		},
		OverdueInstallmentStatus: {
			PaidInstallmentStatus,
			CanceledInstallmentStatus,
		},
	},
	[]InstallmentStatus{
		PaidInstallmentStatus,
		ExpiredInstallmentStatus,
		CanceledInstallmentStatus,
	},
)

// IsValid reports whether the installment status is known by the domain.
func (s InstallmentStatus) IsValid() bool {
	return installmentStatusTransitions.isValid(s)
}

// CanTransitionTo reports whether the installment status may change to next.
func (s InstallmentStatus) CanTransitionTo(next InstallmentStatus) bool {
	return installmentStatusTransitions.canTransitionTo(s, next)
}

// ValidateTransitionTo returns an error when the installment status may not change to next.
func (s InstallmentStatus) ValidateTransitionTo(next InstallmentStatus) error {
	return installmentStatusTransitions.validateTransitionTo(s, next)
}

// IsTerminal reports whether the installment status is terminal.
func (s InstallmentStatus) IsTerminal() bool {
	return installmentStatusTransitions.isTerminal(s)
}

// IsOffered reports whether the installment is still a pre-contract offer.
func (s InstallmentStatus) IsOffered() bool {
	return s == OfferedInstallmentStatus
}

// IsActive reports whether the installment belongs to an active contract schedule.
func (s InstallmentStatus) IsActive() bool {
	switch s {
	case ScheduledInstallmentStatus, DueInstallmentStatus, OverdueInstallmentStatus:
		return true

	default:
		return false
	}
}

// MaxInstallmentGracePeriodDays is the supported reminder lookback for new policy terms.
// Persisted contractual snapshots must never be shortened to this operational bound.
const MaxInstallmentGracePeriodDays uint = 31

// OverdueEligibleAt returns when a due installment becomes overdue eligible.
//
// Contractual grace is a wall-clock duration of GracePeriodDays * 24 hours
// from DueAt (including non-midnight due instants). This matches Collections
// SQL candidate selection and collectionsv1. Reminder policy uses business
// calendar dates for SMS scheduling and is intentionally separate.
func (i Installment) OverdueEligibleAt() time.Time {
	// Add calendar days only after converting to UTC: each UTC day is 24h.
	// This also avoids time.Duration overflow for PostgreSQL integer snapshots.
	return i.DueAt.UTC().AddDate(0, 0, int(i.GracePeriodDays))
}

// IsOverdueEligible reports whether now is at or after the overdue eligibility time.
func (i Installment) IsOverdueEligible(now time.Time) bool {
	return !now.UTC().Before(i.OverdueEligibleAt())
}

// Installment stores a persisted installment offer row for a checkout decision.
type Installment struct {
	// ID uniquely identifies this installment row.
	ID InstallmentID `db:"id" json:"id"`

	// DecisionID references the checkout decision that produced this installment.
	DecisionID DecisionID `db:"decision_id" json:"decision_id"`

	// ContractID optionally references the contract this installment belongs to.
	ContractID *ContractID `db:"contract_id" json:"contract_id,omitempty"`

	// CustomerID is the internal customer profile primary key for this installment.
	CustomerID CustomerID `db:"customer_id" json:"customer_id"`

	// MerchantProfileID identifies the merchant BNPL profile for this installment.
	MerchantProfileID MerchantProfileID `db:"merchant_profile_id" json:"merchant_profile_id"`

	// MerchantBranchProfileID optionally identifies the branch profile used for checkout.
	MerchantBranchProfileID *MerchantBranchProfileID `db:"merchant_branch_profile_id" json:"merchant_branch_profile_id,omitempty"`

	// InstallmentOrder is the one-based position of this installment in the schedule.
	InstallmentOrder int `db:"installment_order" json:"installment_order"`

	// Currency is the currency of all monetary fields on this installment.
	Currency Currency `db:"currency" json:"currency"`

	// PrincipalAmountMinor is the financed principal portion in minor units.
	PrincipalAmountMinor uint64 `db:"principal_amount_minor" json:"principal_amount_minor"`

	// UpfrontAmountMinor is any upfront amount associated with this installment.
	UpfrontAmountMinor uint64 `db:"upfront_amount_minor" json:"upfront_amount_minor"`

	// BaseFeeAmountMinor is the base fee portion in minor units.
	BaseFeeAmountMinor uint64 `db:"base_fee_amount_minor" json:"base_fee_amount_minor"`

	// TotalAmountMinor is the total payable amount for this installment in minor units.
	TotalAmountMinor uint64 `db:"total_amount_minor" json:"total_amount_minor"`

	// DueAt is when this installment becomes due.
	DueAt time.Time `db:"due_at" json:"due_at"`

	// Status is the current lifecycle state of the installment.
	Status InstallmentStatus `db:"status" json:"status"`

	// GracePeriodDays is the contractual grace period snapshot from the repayment policy
	// at offer creation time. It is persisted and never read from runtime configuration.
	GracePeriodDays uint `db:"grace_period_days" json:"grace_period_days,omitempty"`

	// PaidAmountMinor is the amount already paid toward this installment.
	PaidAmountMinor uint64 `db:"paid_amount_minor" json:"paid_amount_minor"`

	// PaidAt is when this installment was fully paid, if applicable.
	PaidAt *time.Time `db:"paid_at" json:"paid_at,omitempty"`

	// CreatedAt is when the installment was created.
	CreatedAt time.Time `db:"created_at" json:"created_at"`

	// UpdatedAt is when the installment was last modified.
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}
