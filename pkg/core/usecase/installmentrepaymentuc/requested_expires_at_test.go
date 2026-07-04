package installmentrepaymentuc

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	adapterclock "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func newInstallmentRepaymentUCForExpiryTests(t *testing.T, now time.Time) *UseCase {
	t.Helper()

	uc, err := New(
		mock.NewPool(t),
		mock.NewCustomerProfile(t),
		mock.NewMerchantProfile(t),
		mock.NewContract(t),
		mock.NewInstallment(t),
		mock.NewInstallmentCharge(t),
		mock.NewPaymentAttempt(t),
		mock.NewOutbox(t),
		mock.NewCheckoutPaymentClient(t),
		adapterclock.NewFixed(now, time.UTC),
		lognoop.New(),
		"bnpl.audit.events.v1",
		mock.NewIdentifierGenerator(t),
		"https://bnpl.example/return",
	)
	require.NoError(t, err)

	return uc
}

func TestRequestedCheckoutPaymentExpiresAt_StableFromCreatedAtPlusTTL(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	createdAt := now.Add(-3 * time.Minute)
	uc := newInstallmentRepaymentUCForExpiryTests(t, now)

	attempt := model.PaymentAttempt{
		ID:        model.PaymentAttemptID(uuid.New()),
		CreatedAt: createdAt,
	}

	first, err := uc.requestedCheckoutPaymentExpiresAt(attempt, now)
	require.NoError(t, err)
	require.Equal(t, createdAt.Add(DefaultPaymentSessionTTL).UTC(), first.UTC())

	// Retry / recovery with a later clock must not extend the requested window.
	later := now.Add(2 * time.Minute)
	second, err := uc.requestedCheckoutPaymentExpiresAt(attempt, later)
	require.NoError(t, err)
	require.True(t, first.Equal(second))
	require.Equal(t, createdAt.Add(DefaultPaymentSessionTTL).UTC(), second.UTC())
}

func TestRequestedCheckoutPaymentExpiresAt_RejectsElapsedWindowWithoutExtending(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, 10, 20, 11, 0, 0, 0, time.UTC)
	now := createdAt.Add(DefaultPaymentSessionTTL).Add(time.Second)
	uc := newInstallmentRepaymentUCForExpiryTests(t, now)

	_, err := uc.requestedCheckoutPaymentExpiresAt(model.PaymentAttempt{
		ID:        model.PaymentAttemptID(uuid.New()),
		CreatedAt: createdAt,
	}, now)
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeConflict))
}

func TestRequestedCheckoutPaymentExpiresAt_NewAttemptGetsOwnExpiry(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	uc := newInstallmentRepaymentUCForExpiryTests(t, now)

	firstCreated := now.Add(-4 * time.Minute)
	secondCreated := now.Add(-1 * time.Minute)

	first, err := uc.requestedCheckoutPaymentExpiresAt(model.PaymentAttempt{
		ID:        model.PaymentAttemptID(uuid.New()),
		CreatedAt: firstCreated,
	}, now)
	require.NoError(t, err)

	second, err := uc.requestedCheckoutPaymentExpiresAt(model.PaymentAttempt{
		ID:        model.PaymentAttemptID(uuid.New()),
		CreatedAt: secondCreated,
	}, now)
	require.NoError(t, err)

	require.Equal(t, firstCreated.Add(DefaultPaymentSessionTTL).UTC(), first.UTC())
	require.Equal(t, secondCreated.Add(DefaultPaymentSessionTTL).UTC(), second.UTC())
	require.False(t, first.Equal(second))
}

func TestRequestedCheckoutPaymentExpiresAt_RequiresCreatedAt(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	uc := newInstallmentRepaymentUCForExpiryTests(t, now)

	_, err := uc.requestedCheckoutPaymentExpiresAt(model.PaymentAttempt{
		ID: model.PaymentAttemptID(uuid.New()),
	}, now)
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeInternalError))
}
