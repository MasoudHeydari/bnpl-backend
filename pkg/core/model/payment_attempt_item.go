package model

import (
	"database/sql/driver"
	"fmt"
	"math/bits"
	"time"

	"github.com/google/uuid"
)

// PaymentAttemptItemID uniquely identifies a PaymentAttemptItem row.
type PaymentAttemptItemID uuid.UUID

// ParsePaymentAttemptItemID parses a PaymentAttemptItemID from its canonical string form.
func ParsePaymentAttemptItemID(value string) (PaymentAttemptItemID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return PaymentAttemptItemID{}, fmt.Errorf("parse payment attempt item id: %w", err)
	}

	return PaymentAttemptItemID(id), nil
}

func (id PaymentAttemptItemID) UUID() uuid.UUID { return uuid.UUID(id) }

func (id PaymentAttemptItemID) String() string { return uuid.UUID(id).String() }

func (id PaymentAttemptItemID) IsZero() bool { return uuid.UUID(id) == uuid.Nil }

func (id PaymentAttemptItemID) MarshalText() ([]byte, error) {
	return uuid.UUID(id).MarshalText()
}

func (id *PaymentAttemptItemID) UnmarshalText(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data); err != nil {
		return err
	}

	*id = PaymentAttemptItemID(u)

	return nil
}

func (id *PaymentAttemptItemID) Scan(src any) error {
	var u uuid.UUID
	if err := u.Scan(src); err != nil {
		return err
	}

	*id = PaymentAttemptItemID(u)

	return nil
}

func (id PaymentAttemptItemID) Value() (driver.Value, error) { return uuid.UUID(id).Value() }

// PaymentAttemptItemType identifies the source domain of a snapshotted payable item.
type PaymentAttemptItemType string

// Known payment attempt item source types.
const (
	PrincipalPaymentAttemptItemType PaymentAttemptItemType = "principal"
	UpfrontPaymentAttemptItemType   PaymentAttemptItemType = "upfront"
	ChargePaymentAttemptItemType    PaymentAttemptItemType = "charge"
)

// PaymentAttemptItemEffect identifies whether an item increases or decreases payable amount.
type PaymentAttemptItemEffect string

// Known payment attempt item effects on payable amount.
const (
	DebitPaymentAttemptItemEffect  PaymentAttemptItemEffect = "debit"
	CreditPaymentAttemptItemEffect PaymentAttemptItemEffect = "credit"
)

// PaymentAttemptItem stores a snapshotted payable obligation line for a payment attempt.
type PaymentAttemptItem struct {
	// ID uniquely identifies this payment attempt item row.
	ID PaymentAttemptItemID `db:"id" json:"id,omitempty"`

	// PaymentAttemptID references the parent payment attempt aggregate.
	PaymentAttemptID PaymentAttemptID `db:"payment_attempt_id" json:"payment_attempt_id,omitempty"`

	// ItemType identifies the source domain of this payable line.
	ItemType PaymentAttemptItemType `db:"item_type" json:"item_type,omitempty"`

	// SourceID references the domain entity that produced this payable line.
	SourceID uuid.UUID `db:"source_id" json:"source_id,omitempty"`

	// AmountMinor is the absolute amount of this line in minor units.
	AmountMinor uint64 `db:"amount_minor" json:"amount_minor,omitempty"`

	// Effect indicates whether this line increases or decreases the payable total.
	Effect PaymentAttemptItemEffect `db:"effect" json:"effect,omitempty"`

	// CreatedAt is when this item snapshot was created.
	CreatedAt time.Time `db:"created_at" json:"created_at,omitempty"`
}

// PayableAmountFromItems returns debit total minus credit total without underflow.
func PayableAmountFromItems(items []PaymentAttemptItem) (uint64, bool) {
	var debits uint64
	var credits uint64

	for _, item := range items {
		if item.AmountMinor == 0 {
			return 0, false
		}

		switch item.Effect {
		case DebitPaymentAttemptItemEffect:
			next, ok := bits.Add64(debits, item.AmountMinor, 0)
			if ok != 0 {
				return 0, false
			}

			debits = next

		case CreditPaymentAttemptItemEffect:
			next, ok := bits.Add64(credits, item.AmountMinor, 0)
			if ok != 0 {
				return 0, false
			}

			credits = next

		default:
			return 0, false
		}
	}

	if credits > debits {
		return 0, false
	}

	return debits - credits, true
}
