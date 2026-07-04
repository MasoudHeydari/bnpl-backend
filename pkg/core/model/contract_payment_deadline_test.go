package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestContract_EffectivePaymentDeadline(t *testing.T) {
	t.Parallel()

	bnplExpiry := time.Date(
		2026,
		7,
		10,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	fcExpiry := time.Date(
		2026,
		7,
		10,
		11,
		30,
		0,
		0,
		time.UTC,
	)

	t.Run("uses bnpl expiry when financial core expiry is missing", func(t *testing.T) {
		t.Parallel()

		contract := Contract{ExpiresAt: bnplExpiry}
		assert.Equal(t, bnplExpiry, contract.EffectivePaymentDeadline())
	})

	t.Run("uses earlier financial core expiry", func(t *testing.T) {
		t.Parallel()

		contract := Contract{
			ExpiresAt:                 bnplExpiry,
			FinancialPaymentExpiresAt: &fcExpiry,
		}
		assert.Equal(t, fcExpiry, contract.EffectivePaymentDeadline())
	})

	t.Run("uses bnpl expiry when financial core expiry is later", func(t *testing.T) {
		t.Parallel()

		later := bnplExpiry.Add(30 * time.Minute)
		contract := Contract{
			ExpiresAt:                 bnplExpiry,
			FinancialPaymentExpiresAt: &later,
		}
		assert.Equal(t, bnplExpiry, contract.EffectivePaymentDeadline())
	})

	t.Run("ignores zero financial core expiry", func(t *testing.T) {
		t.Parallel()

		zero := time.Time{}
		contract := Contract{
			ExpiresAt:                 bnplExpiry,
			FinancialPaymentExpiresAt: &zero,
		}
		assert.Equal(t, bnplExpiry, contract.EffectivePaymentDeadline())
	})
}

func TestContract_ExpiryEligibleAt(t *testing.T) {
	t.Parallel()

	deadline := time.Date(
		2026,
		7,
		10,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	grace := 5 * time.Minute
	contract := Contract{ExpiresAt: deadline}

	assert.Equal(t, deadline.Add(grace), contract.ExpiryEligibleAt(grace))
}

func TestContract_IsExpiryEligible(t *testing.T) {
	t.Parallel()

	deadline := time.Date(
		2026,
		7,
		10,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	grace := 5 * time.Minute
	contract := Contract{ExpiresAt: deadline}

	assert.False(t, contract.IsExpiryEligible(deadline.Add(4*time.Minute), grace))
	assert.True(t, contract.IsExpiryEligible(deadline.Add(5*time.Minute), grace))
}

func TestContract_PaymentOccurredOnTime(t *testing.T) {
	t.Parallel()

	deadline := time.Date(
		2026,
		7,
		10,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	contract := Contract{ExpiresAt: deadline}

	assert.False(t, contract.PaymentOccurredOnTime(time.Time{}))
	assert.True(t, contract.PaymentOccurredOnTime(deadline))
	assert.True(t, contract.PaymentOccurredOnTime(deadline.Add(-time.Second)))
	assert.False(t, contract.PaymentOccurredOnTime(deadline.Add(time.Second)))
}
