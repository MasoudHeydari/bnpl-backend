package v1

import (
	"testing"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy/merchantprovisioning"
	"github.com/stretchr/testify/require"
)

func TestDeclaredLimitsArePreserved(t *testing.T) {
	for _, declaration := range []model.MerchantSelfDeclaration{
		{
			MaxAmountMinor:    370_000_000,
			MaxAmountCurrency: model.IRRCurrency,
			MaxTenureMonths:   7,
		},
		{
			MaxAmountMinor:    42_000_000,
			MaxAmountCurrency: model.IRRCurrency,
			MaxTenureMonths:   2,
		},
	} {
		terms, err := New().Evaluate(declaration)
		if err != nil {
			t.Fatal(err)
		}

		require.Equal(t, declaration, terms.Limits)
		require.Equal(t, []model.Currency{model.IRRCurrency}, terms.Currencies)
	}
}

func TestInvalidDeclarationCannotBecomeAnUnlimitedAccount(t *testing.T) {
	valid := model.MerchantSelfDeclaration{
		MaxAmountMinor:    42_000_000,
		MaxAmountCurrency: model.IRRCurrency,
		MaxTenureMonths:   4,
	}
	cases := map[string]func(*model.MerchantSelfDeclaration){
		"missing amount":       func(d *model.MerchantSelfDeclaration) { d.MaxAmountMinor = 0 },
		"missing tenure":       func(d *model.MerchantSelfDeclaration) { d.MaxTenureMonths = 0 },
		"missing currency":     func(d *model.MerchantSelfDeclaration) { d.MaxAmountCurrency = "" },
		"unsupported currency": func(d *model.MerchantSelfDeclaration) { d.MaxAmountCurrency = model.USDCurrency },
		"amount overflow":      func(d *model.MerchantSelfDeclaration) { d.MaxAmountMinor = 1 << 63 },
		"tenure overflow":      func(d *model.MerchantSelfDeclaration) { d.MaxTenureMonths = 1 << 31 },
	}
	for name, alter := range cases {
		t.Run(name, func(t *testing.T) {
			declaration := valid
			alter(&declaration)
			_, err := New().Evaluate(declaration)
			require.ErrorIs(t, err, merchantprovisioning.ErrInvalidDeclaration)
		})
	}
}

// Pin the actual business ceilings, not just the implementation constants.
func TestPlatformCeilings(t *testing.T) {
	cases := []struct {
		name     string
		amount   uint64
		months   uint
		accepted bool
	}{
		{
			name:     "below both ceilings",
			amount:   4_999_999_999,
			months:   11,
			accepted: true,
		},
		{
			name:     "exact ceilings",
			amount:   5_000_000_000,
			months:   12,
			accepted: true,
		},
		{
			name:   "amount exceeds ceiling",
			amount: 5_000_000_001,
			months: 12,
		},
		{
			name:   "tenure exceeds ceiling",
			amount: 5_000_000_000,
			months: 13,
		},
		{
			name:   "both exceed ceilings",
			amount: 5_000_000_001,
			months: 13,
		},
		{
			name:     "minimum positive values",
			amount:   1,
			months:   1,
			accepted: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			declaration := model.MerchantSelfDeclaration{
				MaxAmountMinor:    tc.amount,
				MaxAmountCurrency: model.IRRCurrency,
				MaxTenureMonths:   tc.months,
			}
			terms, err := New().Evaluate(declaration)
			if tc.accepted {
				require.NoError(t, err)
				require.Equal(t, declaration, terms.Limits)
				return
			}

			require.ErrorIs(t, err, merchantprovisioning.ErrInvalidDeclaration)
			require.Equal(t, merchantprovisioning.Terms{}, terms)
		})
	}
}
