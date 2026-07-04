package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// InstallmentChargeID uniquely identifies an InstallmentCharge.
type InstallmentChargeID uuid.UUID

// ParseInstallmentChargeID parses an InstallmentChargeID from its canonical string form.
func ParseInstallmentChargeID(value string) (InstallmentChargeID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return InstallmentChargeID{}, fmt.Errorf("parse installment charge id: %w", err)
	}

	return InstallmentChargeID(id), nil
}

func (id InstallmentChargeID) UUID() uuid.UUID { return uuid.UUID(id) }

func (id InstallmentChargeID) String() string { return uuid.UUID(id).String() }

func (id InstallmentChargeID) IsZero() bool { return uuid.UUID(id) == uuid.Nil }

func (id InstallmentChargeID) MarshalText() ([]byte, error) { return uuid.UUID(id).MarshalText() }

func (id *InstallmentChargeID) UnmarshalText(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data); err != nil {
		return err
	}

	*id = InstallmentChargeID(u)

	return nil
}

func (id *InstallmentChargeID) Scan(src any) error {
	var u uuid.UUID
	if err := u.Scan(src); err != nil {
		return err
	}

	*id = InstallmentChargeID(u)

	return nil
}

func (id InstallmentChargeID) Value() (driver.Value, error) { return uuid.UUID(id).Value() }

// Installment charge type constants.
const (
	BNPLFeeInstallmentChargeType    = "bnpl_fee"
	CommissionInstallmentChargeType = "commission"
	LateFeeInstallmentChargeType    = "late_fee"
	TaxInstallmentChargeType        = "tax"
	DiscountInstallmentChargeType   = "discount"
	WaiverInstallmentChargeType     = "waiver"
)

// Installment charge status constants.
const (
	OfferedInstallmentChargeStatus  InstallmentChargeStatus = "offered"
	AppliedInstallmentChargeStatus  InstallmentChargeStatus = "applied"
	PaidInstallmentChargeStatus     InstallmentChargeStatus = "paid"
	WaivedInstallmentChargeStatus   InstallmentChargeStatus = "waived"
	CanceledInstallmentChargeStatus InstallmentChargeStatus = "canceled"
)

var installmentChargeStatusTransitions = newStatusTransitionTable(
	"installment charge status",
	map[InstallmentChargeStatus][]InstallmentChargeStatus{
		OfferedInstallmentChargeStatus: {
			AppliedInstallmentChargeStatus,
			PaidInstallmentChargeStatus,
			CanceledInstallmentChargeStatus,
		},
		AppliedInstallmentChargeStatus: {
			PaidInstallmentChargeStatus,
			WaivedInstallmentChargeStatus,
			CanceledInstallmentChargeStatus,
		},
	},
	[]InstallmentChargeStatus{
		PaidInstallmentChargeStatus,
		WaivedInstallmentChargeStatus,
		CanceledInstallmentChargeStatus,
	},
)

// InstallmentChargeType identifies the kind of financial charge on an installment.
type InstallmentChargeType string

// InstallmentChargeStatus identifies the lifecycle state of an installment charge.
type InstallmentChargeStatus string

// IsValid reports whether the installment charge status is known by the domain.
func (s InstallmentChargeStatus) IsValid() bool {
	return installmentChargeStatusTransitions.isValid(s)
}

// CanTransitionTo reports whether the installment charge status may change to next.
func (s InstallmentChargeStatus) CanTransitionTo(next InstallmentChargeStatus) bool {
	return installmentChargeStatusTransitions.canTransitionTo(s, next)
}

// ValidateTransitionTo returns an error when the installment charge status may not change to next.
func (s InstallmentChargeStatus) ValidateTransitionTo(next InstallmentChargeStatus) error {
	return installmentChargeStatusTransitions.validateTransitionTo(s, next)
}

// IsTerminal reports whether the installment charge status is terminal.
func (s InstallmentChargeStatus) IsTerminal() bool {
	return installmentChargeStatusTransitions.isTerminal(s)
}

// IsOffered reports whether the charge is still part of a pre-contract offer.
func (s InstallmentChargeStatus) IsOffered() bool {
	return s == OfferedInstallmentChargeStatus
}

// InstallmentChargePayerType identifies who pays a charge.
type InstallmentChargePayerType string

// InstallmentChargeRecipientType identifies who receives a charge.
type InstallmentChargeRecipientType string

// Known installment charge payer and recipient types.
const (
	CustomerInstallmentChargePayerType     InstallmentChargePayerType     = "customer"
	MerchantInstallmentChargePayerType     InstallmentChargePayerType     = "merchant"
	PlatformInstallmentChargePayerType     InstallmentChargePayerType     = "platform"
	CustomerInstallmentChargeRecipientType InstallmentChargeRecipientType = "customer"
	MerchantInstallmentChargeRecipientType InstallmentChargeRecipientType = "merchant"
	PlatformInstallmentChargeRecipientType InstallmentChargeRecipientType = "platform"
)

// InstallmentCharge stores a persisted charge row associated with an installment.
type InstallmentCharge struct {
	// ID uniquely identifies this installment charge row.
	ID InstallmentChargeID `db:"id" json:"id"`

	// InstallmentID references the installment this charge belongs to.
	InstallmentID InstallmentID `db:"installment_id" json:"installment_id"`

	// ChargeType identifies the business kind of charge.
	ChargeType InstallmentChargeType `db:"charge_type" json:"charge_type"`

	// ChargeStatus is the current lifecycle state of the charge.
	ChargeStatus InstallmentChargeStatus `db:"charge_status" json:"charge_status"`

	// AmountMinor is the charge amount in minor units.
	AmountMinor uint64 `db:"amount_minor" json:"amount_minor"`

	// Currency is the currency of AmountMinor.
	Currency Currency `db:"currency" json:"currency"`

	// PayerType identifies who is responsible for paying this charge.
	PayerType InstallmentChargePayerType `db:"payer_type" json:"payer_type"`

	// RecipientType identifies who receives this charge.
	RecipientType InstallmentChargeRecipientType `db:"recipient_type" json:"recipient_type"`

	// PolicyID identifies the policy that produced this charge.
	PolicyID string `db:"policy_id" json:"policy_id"`

	// PolicyVersion identifies the policy version that produced this charge.
	PolicyVersion string `db:"policy_version" json:"policy_version"`

	// AssessedAt is when the charge was assessed.
	AssessedAt time.Time `db:"assessed_at" json:"assessed_at"`

	// DueAt is when the charge becomes due, if applicable.
	DueAt *time.Time `db:"due_at" json:"due_at,omitempty"`

	// PaidAt is when the charge was paid, if applicable.
	PaidAt *time.Time `db:"paid_at" json:"paid_at,omitempty"`

	// Metadata stores optional serialized charge metadata.
	Metadata json.RawMessage `db:"metadata" json:"metadata"`

	// CreatedAt is when the charge was created.
	CreatedAt time.Time `db:"created_at" json:"created_at"`

	// UpdatedAt is when the charge was last modified.
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// CustomerPaymentChargeEffect reports debit/credit effect for customer-payable charge types.
func (t InstallmentChargeType) CustomerPaymentChargeEffect() (PaymentAttemptItemEffect, error) {
	const op = "model.InstallmentChargeType.CustomerPaymentChargeEffect"

	switch t {
	case BNPLFeeInstallmentChargeType,
		CommissionInstallmentChargeType,
		LateFeeInstallmentChargeType,
		TaxInstallmentChargeType:
		return DebitPaymentAttemptItemEffect, nil

	case DiscountInstallmentChargeType, WaiverInstallmentChargeType:
		return CreditPaymentAttemptItemEffect, nil

	default:
		return "", fmt.Errorf("%s: unsupported customer payable charge type %q", op, t)
	}
}

// IsCustomerPayableCharge reports whether a charge should be included in customer repayment amount.
func IsCustomerPayableCharge(charge InstallmentCharge) bool {
	if charge.PayerType != CustomerInstallmentChargePayerType {
		return false
	}

	switch charge.ChargeStatus {
	case OfferedInstallmentChargeStatus, AppliedInstallmentChargeStatus:
		return true

	default:
		return false
	}
}

// CustomerPayableAmount calculates installment principal plus customer-payable charges.
func CustomerPayableAmount(installment Installment, charges []InstallmentCharge) (uint64, bool, error) {
	items, err := BuildPaymentAttemptItems(installment, charges)
	if err != nil {
		return 0, false, err
	}

	amount, ok := PayableAmountFromItems(items)

	return amount, ok, nil
}

// BuildPaymentAttemptItems snapshots principal, upfront, and customer-payable charges for a payment attempt.
func BuildPaymentAttemptItems(installment Installment, charges []InstallmentCharge) ([]PaymentAttemptItem, error) {
	items := []PaymentAttemptItem{
		{
			ItemType:    PrincipalPaymentAttemptItemType,
			SourceID:    installment.ID.UUID(),
			AmountMinor: installment.PrincipalAmountMinor,
			Effect:      DebitPaymentAttemptItemEffect,
		},
	}

	if installment.UpfrontAmountMinor > 0 {
		items = append(items, PaymentAttemptItem{
			ItemType:    UpfrontPaymentAttemptItemType,
			SourceID:    installment.ID.UUID(),
			AmountMinor: installment.UpfrontAmountMinor,
			Effect:      DebitPaymentAttemptItemEffect,
		})
	}

	for _, charge := range charges {
		if !IsCustomerPayableCharge(charge) {
			continue
		}

		effect, err := charge.ChargeType.CustomerPaymentChargeEffect()
		if err != nil {
			return nil, err
		}

		items = append(items, PaymentAttemptItem{
			ItemType:    ChargePaymentAttemptItemType,
			SourceID:    charge.ID.UUID(),
			AmountMinor: charge.AmountMinor,
			Effect:      effect,
		})
	}

	return items, nil
}
