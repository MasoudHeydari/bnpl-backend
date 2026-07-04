package model

import (
	"errors"
	"math"
)

// MaxAmountMinor is the maximum monetary amount expressible as a signed 64-bit integer.
const MaxAmountMinor uint64 = math.MaxInt64

// Amount validation and arithmetic boundary errors.
var (
	ErrAmountMinorRequired   = errors.New("amount minor must be positive")
	ErrAmountMinorOutOfRange = errors.New("amount minor exceeds supported signed 64-bit range")
	ErrAmountMinorOverflow   = errors.New("amount minor arithmetic overflow")
	ErrAmountMinorUnderflow  = errors.New("amount minor arithmetic underflow")
)

// ValidatePositiveAmountMinor validates monetary values shared by Go, PostgreSQL BIGINT,
// and Financial Core signed 64-bit boundaries.
func ValidatePositiveAmountMinor(amount uint64) error {
	if amount == 0 {
		return ErrAmountMinorRequired
	}

	if amount > MaxAmountMinor {
		return ErrAmountMinorOutOfRange
	}

	return nil
}

// ValidateNonNegativeAmountMinor validates an optional amount against the shared signed boundary.
func ValidateNonNegativeAmountMinor(amount uint64) error {
	if amount > MaxAmountMinor {
		return ErrAmountMinorOutOfRange
	}

	return nil
}

// AmountMinorInt64 converts a validated amount to int64 for external APIs.
func AmountMinorInt64(amount uint64) (int64, error) {
	if err := ValidateNonNegativeAmountMinor(amount); err != nil {
		return 0, err
	}

	// MaxAmountMinor == math.MaxInt64; re-check so converters see a direct bound.
	if amount > math.MaxInt64 {
		return 0, ErrAmountMinorOutOfRange
	}

	return int64(amount), nil
}

// AddAmountMinor adds two monetary values while enforcing the signed BIGINT boundary.
func AddAmountMinor(a, b uint64) (uint64, error) {
	if err := ValidateNonNegativeAmountMinor(a); err != nil {
		return 0, err
	}

	if err := ValidateNonNegativeAmountMinor(b); err != nil {
		return 0, err
	}

	if a > MaxAmountMinor-b {
		return 0, ErrAmountMinorOverflow
	}

	return a + b, nil
}

// SubtractAmountMinor subtracts b from a without permitting underflow.
func SubtractAmountMinor(a, b uint64) (uint64, error) {
	if err := ValidateNonNegativeAmountMinor(a); err != nil {
		return 0, err
	}

	if err := ValidateNonNegativeAmountMinor(b); err != nil {
		return 0, err
	}

	if a < b {
		return 0, ErrAmountMinorUnderflow
	}

	return a - b, nil
}

// SumAmountMinor sums monetary values while enforcing the signed BIGINT boundary.
func SumAmountMinor(values ...uint64) (uint64, error) {
	var total uint64
	for _, value := range values {
		var err error
		total, err = AddAmountMinor(total, value)
		if err != nil {
			return 0, err
		}
	}

	return total, nil
}
