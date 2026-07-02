package model

// Money represents an amount in the smallest unit of a currency.
type Money struct {
	// AmountMinor is the amount in the smallest unit of Currency.
	AmountMinor uint64 `db:"amount_minor" json:"amount_minor,omitempty"`

	// Currency identifies the currency of AmountMinor.
	Currency Currency `db:"currency" json:"currency,omitempty"`
}
