//go:build e2e

package e2e_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	extcheckoutpayment "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
)

func validInstallmentCreateCheckoutInput() extcheckoutpayment.CreateCheckoutPaymentInput {
	version := int64(1)

	return extcheckoutpayment.CreateCheckoutPaymentInput{
		IdempotencyKey:         "e2e-idem-fp-1",
		SourceType:             "bnpl_payment_attempt",
		SourceID:               "attempt-fp-1",
		SourceVersion:          &version,
		PayerType:              extcheckoutpayment.PartyCustomer,
		PayerID:                "customer-1",
		PayeeType:              extcheckoutpayment.PartyPlatform,
		PayeeID:                "",
		AmountMinor:            100000,
		Currency:               "IRR",
		ProviderCode:           "provider",
		CollectionObligationID: "co-1",
		SuccessReturnURL:       "https://bnpl.example/ok",
		FailureReturnURL:       "https://bnpl.example/fail",
		CancelReturnURL:        "https://bnpl.example/cancel",
		Description:            "BNPL installment repayment",
		ExpiresAt:              time.Date(2026, 10, 20, 12, 15, 0, 0, time.UTC),
	}
}

func TestE2E_FCFake_RequestFingerprint_PartyTypeCanonicalization(t *testing.T) {
	t.Parallel()

	base := validInstallmentCreateCheckoutInput()
	canonical := createCheckoutPaymentRequestFingerprint(base)

	mixed := base
	mixed.PayerType = " Customer "
	mixed.PayeeType = "PLATFORM"
	require.Equal(t, canonical, createCheckoutPaymentRequestFingerprint(mixed))

	withDescSpace := base
	withDescSpace.Description = base.Description + " "
	require.NotEqual(t, canonical, createCheckoutPaymentRequestFingerprint(withDescSpace))
}

func TestE2E_FCFake_RequestFingerprint_CollisionSafeEncoding(t *testing.T) {
	t.Parallel()

	base := validInstallmentCreateCheckoutInput()
	fp1 := createCheckoutPaymentRequestFingerprint(base)
	fp2 := createCheckoutPaymentRequestFingerprint(base)
	require.Equal(t, fp1, fp2)

	withUnitSep := base
	withUnitSep.Description = "partA\x1fpartB"
	fpSep := createCheckoutPaymentRequestFingerprint(withUnitSep)
	require.NotEqual(t, fp1, fpSep)

	// Adjacent-field rearrangement that would collide under delimiter joining.
	rearranged := base
	rearranged.PayerID = base.PayerID + base.PayeeType
	rearranged.PayeeType = ""
	fpRearranged := createCheckoutPaymentRequestFingerprint(rearranged)
	require.NotEqual(t, fp1, fpRearranged)

	nilVersion := base
	nilVersion.SourceVersion = nil
	fpNil := createCheckoutPaymentRequestFingerprint(nilVersion)
	zero := int64(0)
	zeroVersion := base
	zeroVersion.SourceVersion = &zero
	fpZero := createCheckoutPaymentRequestFingerprint(zeroVersion)
	require.NotEqual(t, fpNil, fpZero)

	later := base
	later.ExpiresAt = base.ExpiresAt.Add(time.Second)
	require.NotEqual(t, fp1, createCheckoutPaymentRequestFingerprint(later))

	// Equivalent UTC instants (same instant, different Location presentation).
	loc := time.FixedZone("UTC+3:30", 3*3600+30*60)
	sameInstant := base
	sameInstant.ExpiresAt = base.ExpiresAt.In(loc)
	require.Equal(t, fp1, createCheckoutPaymentRequestFingerprint(sameInstant))
}

func TestE2E_FCFake_IdempotencyFingerprint_DelimiterInFieldsStillConflicts(t *testing.T) {
	t.Parallel()

	fc := newControllableCheckoutPaymentClient()
	ctx := context.Background()
	base := validInstallmentCreateCheckoutInput()
	base.Description = "a\x1fb"
	base.IdempotencyKey = "e2e-idem-fp-sep"

	_, err := fc.CreateCheckoutPayment(ctx, base)
	require.NoError(t, err)

	modified := base
	modified.Description = "a"
	modified.ProviderCode = "\x1fb" + modified.ProviderCode
	_, err = fc.CreateCheckoutPayment(ctx, modified)
	require.Error(t, err)

	var classified *extcheckoutpayment.ClassifiedFailure
	require.ErrorAs(t, err, &classified)
	require.Equal(t, extcheckoutpayment.FailureClassPermanentConflict, classified.Class)
	require.Equal(t, 1, fc.committedPaymentCount())
}

func TestE2E_FCFake_IdempotencyFingerprint_ExactReplay(t *testing.T) {
	t.Parallel()

	fc := newControllableCheckoutPaymentClient()
	ctx := context.Background()
	in := validInstallmentCreateCheckoutInput()

	first, err := fc.CreateCheckoutPayment(ctx, in)
	require.NoError(t, err)
	require.False(t, first.ReusedIdempotentResult)
	require.Equal(t, 1, fc.committedPaymentCount())

	replay, err := fc.CreateCheckoutPayment(ctx, in)
	require.NoError(t, err)
	require.True(t, replay.ReusedIdempotentResult)
	require.Equal(t, first.PaymentIntentID, replay.PaymentIntentID)
	require.Equal(t, first.PaymentSessionID, replay.PaymentSessionID)
	require.True(t, first.ExpiresAt.UTC().Equal(replay.ExpiresAt.UTC()))
	require.Equal(t, 1, fc.committedPaymentCount())
}

func TestE2E_FCFake_IdempotencyFingerprint_MismatchesConflict(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	base := validInstallmentCreateCheckoutInput()

	tests := []struct {
		name   string
		mutate func(*extcheckoutpayment.CreateCheckoutPaymentInput)
	}{
		{
			name: "amount",
			mutate: func(in *extcheckoutpayment.CreateCheckoutPaymentInput) {
				in.AmountMinor = 120000
			},
		},
		{
			name: "currency",
			mutate: func(in *extcheckoutpayment.CreateCheckoutPaymentInput) {
				in.Currency = "USD"
			},
		},
		{
			name: "expires_at",
			mutate: func(in *extcheckoutpayment.CreateCheckoutPaymentInput) {
				in.ExpiresAt = in.ExpiresAt.Add(time.Minute)
			},
		},
		{
			name: "payer",
			mutate: func(in *extcheckoutpayment.CreateCheckoutPaymentInput) {
				in.PayerID = "customer-2"
			},
		},
		{
			name: "payee type",
			mutate: func(in *extcheckoutpayment.CreateCheckoutPaymentInput) {
				// Distinct payee_type in the fingerprint while remaining a
				// syntactically non-empty party_type value rejected before commit.
				in.PayeeType = "merchant"
			},
		},
		{
			name: "description",
			mutate: func(in *extcheckoutpayment.CreateCheckoutPaymentInput) {
				in.Description = "different description"
			},
		},
		{
			name: "description trailing whitespace",
			mutate: func(in *extcheckoutpayment.CreateCheckoutPaymentInput) {
				in.Description = in.Description + " "
			},
		},
		{
			name: "collection obligation",
			mutate: func(in *extcheckoutpayment.CreateCheckoutPaymentInput) {
				in.CollectionObligationID = "co-2"
			},
		},
		{
			name: "return url",
			mutate: func(in *extcheckoutpayment.CreateCheckoutPaymentInput) {
				in.SuccessReturnURL = "https://bnpl.example/other"
			},
		},
		{
			name: "return url trailing whitespace",
			mutate: func(in *extcheckoutpayment.CreateCheckoutPaymentInput) {
				in.SuccessReturnURL = in.SuccessReturnURL + " "
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fc := newControllableCheckoutPaymentClient()
			_, err := fc.CreateCheckoutPayment(ctx, base)
			require.NoError(t, err)
			require.Equal(t, 1, fc.committedPaymentCount())

			modified := base
			tt.mutate(&modified)
			_, err = fc.CreateCheckoutPayment(ctx, modified)
			require.Error(t, err)
			require.Equal(t, 1, fc.committedPaymentCount(), "must not create a second committed FC payment")

			var classified *extcheckoutpayment.ClassifiedFailure
			if errors.As(err, &classified) {
				require.Equal(t, extcheckoutpayment.FailureClassPermanentConflict, classified.Class)
			}
		})
	}
}

func TestE2E_FCFake_IdempotencyFingerprint_RequiresIdempotencyKey(t *testing.T) {
	t.Parallel()

	fc := newControllableCheckoutPaymentClient()
	in := validInstallmentCreateCheckoutInput()
	in.IdempotencyKey = ""

	_, err := fc.CreateCheckoutPayment(context.Background(), in)
	require.Error(t, err)
	require.Contains(t, err.Error(), "idempotency key is required")
	require.Equal(t, 0, fc.committedPaymentCount())
}

func TestE2E_FCFake_IdempotencyFingerprint_LostResponseThenIdenticalReplay(t *testing.T) {
	t.Parallel()

	fc := newControllableCheckoutPaymentClient()
	fc.setPaymentLostAfterCommit()
	ctx := context.Background()
	in := validInstallmentCreateCheckoutInput()

	_, err := fc.CreateCheckoutPayment(ctx, in)
	require.Error(t, err)
	require.True(t, errors.Is(err, context.DeadlineExceeded))
	require.Equal(t, 1, fc.committedPaymentCount())

	fc.setPaymentSuccess()
	replay, err := fc.CreateCheckoutPayment(ctx, in)
	require.NoError(t, err)
	require.True(t, replay.ReusedIdempotentResult)
	require.Equal(t, 1, fc.committedPaymentCount())
}

func TestE2E_FCFake_IdempotencyFingerprint_LostResponseThenModifiedReplayConflicts(t *testing.T) {
	t.Parallel()

	fc := newControllableCheckoutPaymentClient()
	fc.setPaymentLostAfterCommit()
	ctx := context.Background()
	in := validInstallmentCreateCheckoutInput()

	_, err := fc.CreateCheckoutPayment(ctx, in)
	require.Error(t, err)
	require.Equal(t, 1, fc.committedPaymentCount())

	modified := in
	modified.AmountMinor = 120000
	_, err = fc.CreateCheckoutPayment(ctx, modified)
	require.Error(t, err)

	var classified *extcheckoutpayment.ClassifiedFailure
	require.ErrorAs(t, err, &classified)
	require.Equal(t, extcheckoutpayment.FailureClassPermanentConflict, classified.Class)
	require.Equal(t, 1, fc.committedPaymentCount())
}

func TestE2E_FCFake_IdempotencyFingerprint_ConcurrentIdenticalRequestsOneCommit(t *testing.T) {
	t.Parallel()

	fc := newControllableCheckoutPaymentClient()
	ctx := context.Background()
	in := validInstallmentCreateCheckoutInput()

	const workers = 8
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	results := make(chan extcheckoutpayment.CreateCheckoutPaymentResult, workers)

	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			got, err := fc.CreateCheckoutPayment(ctx, in)
			if err != nil {
				errs <- err
				return
			}

			results <- got
		}()
	}

	wg.Wait()
	close(errs)
	close(results)

	for err := range errs {
		require.NoError(t, err)
	}

	var first extcheckoutpayment.CreateCheckoutPaymentResult
	n := 0
	for got := range results {
		if n == 0 {
			first = got
		} else {
			require.Equal(t, first.PaymentIntentID, got.PaymentIntentID)
			require.Equal(t, first.PaymentSessionID, got.PaymentSessionID)
		}

		n++
	}

	require.Equal(t, workers, n)
	require.Equal(t, 1, fc.committedPaymentCount())
}
