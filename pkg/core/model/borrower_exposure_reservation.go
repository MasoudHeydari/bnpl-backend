package model

import (
	"time"

	"github.com/google/uuid"
)

// BorrowerExposureReservationStatus identifies the lifecycle state of a
// financed exposure reservation.
type BorrowerExposureReservationStatus string

const (
	// BorrowerExposureReservationReserved indicates exposure is reserved for a
	// checkout offer.
	BorrowerExposureReservationReserved BorrowerExposureReservationStatus = "reserved"

	// BorrowerExposureReservationConfirmed indicates the reservation was
	// confirmed after first installment payment.
	BorrowerExposureReservationConfirmed BorrowerExposureReservationStatus = "confirmed"

	// BorrowerExposureReservationReleased indicates the reservation was released
	// without converting to active exposure.
	BorrowerExposureReservationReleased BorrowerExposureReservationStatus = "released"

	// BorrowerExposureReservationExpired indicates the reservation expired before
	// acceptance or payment.
	BorrowerExposureReservationExpired BorrowerExposureReservationStatus = "expired"
)

// BorrowerExposureReservation reserves financed exposure for a checkout decision.
type BorrowerExposureReservation struct {
	ID uuid.UUID `db:"id" json:"id"`

	CheckoutIntentID  uuid.UUID `db:"checkout_intent_id" json:"checkout_intent_id"`
	DecisionID        uuid.UUID `db:"decision_id" json:"decision_id"`
	BorrowerProfileID uuid.UUID `db:"borrower_profile_id" json:"borrower_profile_id"`

	AmountMinor uint64   `db:"amount_minor" json:"amount_minor"`
	Currency    Currency `db:"currency" json:"currency,omitempty"`

	Status    BorrowerExposureReservationStatus `db:"status" json:"status,omitempty"`
	ExpiresAt time.Time                         `db:"expires_at" json:"expires_at"`
	CreatedAt time.Time                         `db:"created_at" json:"created_at"`
	UpdatedAt time.Time                         `db:"updated_at" json:"updated_at"`
}
