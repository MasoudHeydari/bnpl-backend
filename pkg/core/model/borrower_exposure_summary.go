package model

import (
	"time"

	"github.com/google/uuid"
)

// BorrowerExposureSummary tracks active and reserved financed exposure for a
// borrower profile in a single currency.
type BorrowerExposureSummary struct {
	BorrowerProfileID uuid.UUID `db:"borrower_profile_id" json:"borrower_profile_id"`
	Currency          Currency  `db:"currency" json:"currency,omitempty"`

	ActiveExposureAmountMinor   uint64 `db:"active_exposure_amount_minor" json:"active_exposure_amount_minor"`
	ReservedExposureAmountMinor uint64 `db:"reserved_exposure_amount_minor" json:"reserved_exposure_amount_minor"`

	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}
