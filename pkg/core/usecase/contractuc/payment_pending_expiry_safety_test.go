package contractuc

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestPaymentPendingContractSafeToExpire(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	contractID := model.ContractID(uuid.New())
	attemptID := model.PaymentAttemptID(uuid.New())
	contractRow := samplePaymentPendingContract(time.Now().UTC())
	contractRow.ID = contractID

	cases := []struct {
		name  string
		state model.PaymentAttemptState
		want  bool
	}{
		{name: "created is safe", state: model.CreatedPaymentAttemptState, want: true},
		{name: "failed is safe", state: model.FailedPaymentAttemptState, want: true},
		{name: "canceled is safe", state: model.CanceledPaymentAttemptState, want: true},
		{name: "submitted is not safe", state: model.SubmittedPaymentAttemptState, want: false},
		{name: "reconciliation required is not safe", state: model.ReconciliationRequiredPaymentAttemptState, want: false},
		{name: "manual review required is not safe", state: model.ManualReviewRequiredPaymentAttemptState, want: false},
		{name: "succeeded is not safe", state: model.SucceededPaymentAttemptState, want: false},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			deps := newExpirePaymentPendingDeps(t, time.Now().UTC())
			uc := deps.newUseCase(t, 5*time.Minute, 100)
			attempt := model.PaymentAttempt{
				ID:    attemptID,
				State: tt.state,
			}
			deps.paymentAttemptTx.EXPECT().GetByContractIDAndPurposeWithItems(
				ctx, contractID, model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
			).Return(attempt, nil).Once()
			deps.paymentAttemptTx.EXPECT().LockByIDWithItems(ctx, attemptID).Return(attempt, nil).Once()

			got, err := uc.paymentPendingContractSafeToExpire(ctx, deps.paymentAttemptTx, deps.decisionTx, contractRow)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestPaymentPendingContractSafeToExpire_NoAttemptMerchantFundingIsUnsafe(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	deps := newExpirePaymentPendingDeps(t, time.Now().UTC())
	uc := deps.newUseCase(t, 5*time.Minute, 100)
	contractRow := samplePaymentPendingContract(time.Now().UTC())
	decisionID := contractRow.DecisionID

	deps.paymentAttemptTx.EXPECT().GetByContractIDAndPurposeWithItems(
		ctx, contractRow.ID, model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
	).Return(model.PaymentAttempt{}, cerr.ResourceNotFound(nil)).Once()
	deps.decisionTx.EXPECT().GetByID(ctx, decisionID).Return(model.Decision{
		ID:            decisionID,
		PolicyID:      "checkout_contract_approval",
		PolicyVersion: "v2",
	}, nil).Once()

	got, err := uc.paymentPendingContractSafeToExpire(ctx, deps.paymentAttemptTx, deps.decisionTx, contractRow)
	require.NoError(t, err)
	require.False(t, got, "merchant-funding payment_pending without PaymentAttempt must not expire")
}

func TestPaymentPendingContractSafeToExpire_NoAttemptInitialPaymentIsSafe(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	deps := newExpirePaymentPendingDeps(t, time.Now().UTC())
	uc := deps.newUseCase(t, 5*time.Minute, 100)
	contractRow := samplePaymentPendingContract(time.Now().UTC())
	decisionID := contractRow.DecisionID

	deps.paymentAttemptTx.EXPECT().GetByContractIDAndPurposeWithItems(
		ctx, contractRow.ID, model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
	).Return(model.PaymentAttempt{}, cerr.ResourceNotFound(nil)).Once()
	deps.decisionTx.EXPECT().GetByID(ctx, decisionID).Return(model.Decision{
		ID:            decisionID,
		PolicyID:      "checkout_contract_approval",
		PolicyVersion: "v1",
	}, nil).Once()

	got, err := uc.paymentPendingContractSafeToExpire(ctx, deps.paymentAttemptTx, deps.decisionTx, contractRow)
	require.NoError(t, err)
	require.True(t, got, "initial-payment orphan without PaymentAttempt may expire")
}

func TestExpireDuePaymentPendingContracts_SubmittedAttemptDoesNotReleaseGate(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
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
	now := deadline.Add(grace)
	deps := newExpirePaymentPendingDeps(t, now)
	contractRow := samplePaymentPendingContract(deadline)
	attemptID := model.PaymentAttemptID(uuid.New())
	attempt := model.PaymentAttempt{
		ID:    attemptID,
		State: model.SubmittedPaymentAttemptState,
	}

	deps.expectConnTx(ctx)
	deps.contractTx.EXPECT().ListDuePaymentPendingContractIDs(ctx, now, grace, 100).
		Return([]model.ContractID{contractRow.ID}, nil).Once()
	deps.expectExpireConnTx(ctx)
	deps.contractTx.EXPECT().LockByID(ctx, contractRow.ID).Return(contractRow, nil).Once()
	deps.paymentAttemptTx.EXPECT().GetByContractIDAndPurposeWithItems(
		ctx, contractRow.ID, model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
	).Return(attempt, nil).Once()
	deps.paymentAttemptTx.EXPECT().LockByIDWithItems(ctx, attemptID).Return(attempt, nil).Once()

	uc := deps.newUseCase(t, grace, 100)

	output, err := uc.ExpireDuePaymentPendingContracts(ctx)
	require.NoError(t, err)
	require.Zero(t, output.ExpiredContractCount)
}
