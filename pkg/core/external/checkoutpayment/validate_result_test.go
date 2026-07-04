package checkoutpayment

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func validRequiresActionResult(now time.Time) CreateCheckoutPaymentResult {
	return CreateCheckoutPaymentResult{
		PaymentIntentID:  "pi-1",
		PaymentSessionID: "ps-1",
		Status:           RequiresActionPaymentStatus,
		AmountMinor:      1000,
		Currency:         "IRR",
		PaymentLink:      "https://pay.example/ps-1",
		ExpiresAt:        now.Add(10 * time.Minute),
	}
}

func TestValidateRequiresActionCreateResult_AcceptsUsableSession(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	require.NoError(t, ValidateRequiresActionCreateResult(
		validRequiresActionResult(now),
		RequiresActionCreateResultCheck{
			ExpectedAmountMinor: 1000,
			ExpectedCurrency:    "IRR",
			Now:                 now,
		},
	))
}

func TestValidateRequiresActionCreateResult_RejectsInvalidSessions(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	latest := now.Add(15 * time.Minute)

	tests := []struct {
		name   string
		mutate func(*CreateCheckoutPaymentResult)
		check  RequiresActionCreateResultCheck
		want   error
	}{
		{
			name: "missing payment intent",
			mutate: func(r *CreateCheckoutPaymentResult) {
				r.PaymentIntentID = ""
			},
			want: ErrIncompletePaymentIdentifiers,
		},
		{
			name: "missing payment session",
			mutate: func(r *CreateCheckoutPaymentResult) {
				r.PaymentSessionID = ""
			},
			want: ErrIncompletePaymentIdentifiers,
		},
		{
			name: "missing payment link",
			mutate: func(r *CreateCheckoutPaymentResult) {
				r.PaymentLink = ""
			},
			want: ErrInvalidPaymentRedirectURL,
		},
		{
			name: "http url",
			mutate: func(r *CreateCheckoutPaymentResult) {
				r.PaymentLink = "http://pay.example/ps-1"
			},
			want: ErrInvalidPaymentRedirectURL,
		},
		{
			name: "malformed url",
			mutate: func(r *CreateCheckoutPaymentResult) {
				r.PaymentLink = "://bad"
			},
			want: ErrInvalidPaymentRedirectURL,
		},
		{
			name: "url with userinfo",
			mutate: func(r *CreateCheckoutPaymentResult) {
				r.PaymentLink = "https://user:pass@pay.example/ps-1"
			},
			want: ErrInvalidPaymentRedirectURL,
		},
		{
			name: "missing expiry",
			mutate: func(r *CreateCheckoutPaymentResult) {
				r.ExpiresAt = time.Time{}
			},
			want: ErrMissingPaymentExpiry,
		},
		{
			name: "already expired",
			mutate: func(r *CreateCheckoutPaymentResult) {
				r.ExpiresAt = now.Add(-time.Second)
			},
			want: ErrPaymentExpiryNotUsable,
		},
		{
			name: "expiry equal to now",
			mutate: func(r *CreateCheckoutPaymentResult) {
				r.ExpiresAt = now
			},
			want: ErrPaymentExpiryNotUsable,
		},
		{
			name: "expiry after allowed window",
			mutate: func(r *CreateCheckoutPaymentResult) {
				r.ExpiresAt = latest.Add(time.Second)
			},
			check: RequiresActionCreateResultCheck{
				ExpectedAmountMinor:    1000,
				ExpectedCurrency:       "IRR",
				Now:                    now,
				LatestAllowedExpiresAt: &latest,
			},
			want: ErrPaymentExpiryNotUsable,
		},
		{
			name: "amount mismatch",
			mutate: func(r *CreateCheckoutPaymentResult) {
				r.AmountMinor = 999
			},
			want: ErrAmountCurrencyMismatch,
		},
		{
			name: "currency mismatch",
			mutate: func(r *CreateCheckoutPaymentResult) {
				r.Currency = "USD"
			},
			want: ErrAmountCurrencyMismatch,
		},
		{
			name: "unsupported succeeded status",
			mutate: func(r *CreateCheckoutPaymentResult) {
				r.Status = SucceededPaymentStatus
			},
			want: ErrUnsupportedPaymentStatus,
		},
		{
			name: "unsupported processing status",
			mutate: func(r *CreateCheckoutPaymentResult) {
				r.Status = ProcessingPaymentStatus
			},
			want: ErrUnsupportedPaymentStatus,
		},
		{
			name: "unsupported failed status",
			mutate: func(r *CreateCheckoutPaymentResult) {
				r.Status = FailedPaymentStatus
			},
			want: ErrUnsupportedPaymentStatus,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := validRequiresActionResult(now)
			tt.mutate(&result)
			check := tt.check
			if check.ExpectedAmountMinor == 0 && check.ExpectedCurrency == "" {
				check = RequiresActionCreateResultCheck{
					ExpectedAmountMinor: 1000,
					ExpectedCurrency:    "IRR",
					Now:                 now,
				}
			}

			err := ValidateRequiresActionCreateResult(result, check)
			require.Error(t, err)
			require.True(t, errors.Is(err, tt.want), "got %v want %v", err, tt.want)
		})
	}
}

func TestValidatePaymentRedirectURL_RejectsOversized(t *testing.T) {
	t.Parallel()

	oversized := "https://pay.example/" + strings.Repeat("a", DefaultMaxPaymentRedirectURLLength)
	require.Error(t, ValidatePaymentRedirectURL(oversized, DefaultMaxPaymentRedirectURLLength))
}

func TestValidateActionablePaymentSession(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	valid := ActionablePaymentSession{
		PaymentIntentID:  "pi-1",
		PaymentSessionID: "ps-1",
		PaymentURL:       "https://pay.example/ps-1",
		ExpiresAt:        now.Add(10 * time.Minute),
	}

	require.NoError(t, ValidateActionablePaymentSession(valid, now, 0, 0))

	tests := []struct {
		name   string
		mutate func(*ActionablePaymentSession)
		want   error
	}{
		{
			name: "missing intent",
			mutate: func(s *ActionablePaymentSession) {
				s.PaymentIntentID = ""
			},
			want: ErrIncompletePaymentIdentifiers,
		},
		{
			name: "missing session",
			mutate: func(s *ActionablePaymentSession) {
				s.PaymentSessionID = ""
			},
			want: ErrIncompletePaymentIdentifiers,
		},
		{
			name: "missing url",
			mutate: func(s *ActionablePaymentSession) {
				s.PaymentURL = ""
			},
			want: ErrInvalidPaymentRedirectURL,
		},
		{
			name: "http url",
			mutate: func(s *ActionablePaymentSession) {
				s.PaymentURL = "http://pay.example/ps-1"
			},
			want: ErrInvalidPaymentRedirectURL,
		},
		{
			name: "url with userinfo",
			mutate: func(s *ActionablePaymentSession) {
				s.PaymentURL = "https://user:pass@pay.example/ps-1"
			},
			want: ErrInvalidPaymentRedirectURL,
		},
		{
			name: "missing expiry",
			mutate: func(s *ActionablePaymentSession) {
				s.ExpiresAt = time.Time{}
			},
			want: ErrMissingPaymentExpiry,
		},
		{
			name: "expiry equal now",
			mutate: func(s *ActionablePaymentSession) {
				s.ExpiresAt = now
			},
			want: ErrPaymentExpiryNotUsable,
		},
		{
			name: "expiry in past",
			mutate: func(s *ActionablePaymentSession) {
				s.ExpiresAt = now.Add(-time.Second)
			},
			want: ErrPaymentExpiryNotUsable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			session := valid
			tt.mutate(&session)
			err := ValidateActionablePaymentSession(session, now, 0, 0)
			require.Error(t, err)
			require.True(t, errors.Is(err, tt.want), "got %v want %v", err, tt.want)
		})
	}
}
