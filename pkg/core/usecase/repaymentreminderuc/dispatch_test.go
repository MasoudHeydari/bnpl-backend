package repaymentreminderuc_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	adapterclock "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/identifier/uuidgen"
	metricsnoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/metrics/noop"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy/shortlinkdestination"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/repaymentreminderuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/shortlinkuc"
)

func TestDispatch_PagesBeyondFirstCustomerBatch(t *testing.T) {
	t.Parallel()

	loc := time.UTC
	now := time.Date(2026, time.March, 10, 10, 0, 0, 0, loc)
	businessDate := time.Date(2026, time.March, 10, 0, 0, 0, 0, loc)

	customerA := model.CustomerID(uuid.MustParse("00000000-0000-0000-0000-00000000000a"))
	customerB := model.CustomerID(uuid.MustParse("00000000-0000-0000-0000-00000000000b"))
	extA := model.ExternalCustomerID(uuid.New())
	extB := model.ExternalCustomerID(uuid.New())
	installmentA := model.InstallmentID(uuid.New())
	installmentB := model.InstallmentID(uuid.New())
	contractA := model.ContractID(uuid.New())
	contractB := model.ContractID(uuid.New())

	dueToday := businessDate.UTC()

	page1 := []repo.ReminderCandidate{
		{
			Installment: model.Installment{
				ID:              installmentA,
				ContractID:      &contractA,
				CustomerID:      customerA,
				DueAt:           dueToday,
				GracePeriodDays: 5,
				Status:          model.DueInstallmentStatus,
			},
			ExternalCustomerID: extA,
		},
	}
	page2 := []repo.ReminderCandidate{
		{
			Installment: model.Installment{
				ID:              installmentB,
				ContractID:      &contractB,
				CustomerID:      customerB,
				DueAt:           dueToday,
				GracePeriodDays: 5,
				Status:          model.DueInstallmentStatus,
			},
			ExternalCustomerID: extB,
		},
	}

	pool := mock.NewPool(t)
	conn := mock.NewConn(t)
	tx := mock.NewTx(t)
	installment := mock.NewInstallment(t)
	installmentConnQ := mock.NewInstallmentConnQueryer(t)
	installmentTxQ := mock.NewInstallmentTxQueryer(t)
	reminder := mock.NewRepaymentReminder(t)
	reminderTxQ := mock.NewRepaymentReminderTxQueryer(t)
	paymentAttempt := mock.NewPaymentAttempt(t)
	paymentAttemptTxQ := mock.NewPaymentAttemptTxQueryer(t)
	outbox := mock.NewOutbox(t)
	outboxTxQ := mock.NewOutboxTxQueryer(t)
	shortLinkRepo := mock.NewShortLink(t)
	shortLinkTxQ := mock.NewShortLinkTxQueryer(t)

	var listCalls int
	pool.EXPECT().Conn(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(ctx context.Context, fn repo.ConnHandler) error {
			return fn(ctx, conn)
		})
	conn.EXPECT().Tx(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(ctx context.Context, fn repo.TxHandler) error {
			return fn(ctx, tx)
		}).Maybe()

	installment.EXPECT().Conn(conn).Return(installmentConnQ)
	installmentConnQ.EXPECT().
		ListReminderCandidatePage(
			testifymock.Anything,
			testifymock.Anything,
			testifymock.Anything,
			testifymock.MatchedBy(func(d time.Time) bool {
				return d.Year() == businessDate.Year() &&
					d.Month() == businessDate.Month() &&
					d.Day() == businessDate.Day()
			}),
			testifymock.Anything,
			1,
		).
		RunAndReturn(func(
			_ context.Context,
			_ time.Time,
			_ time.Time,
			_ time.Time,
			after model.CustomerID,
			_ int,
		) ([]repo.ReminderCandidate, error) {
			listCalls++
			switch listCalls {
			case 1:
				require.True(t, after.IsZero())
				return page1, nil

			case 2:
				require.Equal(t, customerA, after)
				return page2, nil

			default:
				require.Equal(t, customerB, after)
				return nil, nil
			}
		})

	installment.EXPECT().Tx(tx).Return(installmentTxQ)
	reminder.EXPECT().Tx(tx).Return(reminderTxQ)
	paymentAttempt.EXPECT().Tx(tx).Return(paymentAttemptTxQ)
	outbox.EXPECT().Tx(tx).Return(outboxTxQ)
	shortLinkRepo.EXPECT().Tx(tx).Return(shortLinkTxQ)

	reminderTxQ.EXPECT().
		LockByCustomerBusinessDate(testifymock.Anything, testifymock.Anything, testifymock.Anything).
		Return(model.RepaymentReminder{}, cerr.ResourceNotFound(nil)).Times(2)

	installmentTxQ.EXPECT().
		ListUnpaidByCustomerID(testifymock.Anything, customerA, testifymock.Anything).
		Return([]model.Installment{page1[0].Installment}, nil)
	installmentTxQ.EXPECT().
		ListUnpaidByCustomerID(testifymock.Anything, customerB, testifymock.Anything).
		Return([]model.Installment{page2[0].Installment}, nil)

	paymentAttemptTxQ.EXPECT().
		ListInstallmentIDsWithNonTerminalAttempts(testifymock.Anything, testifymock.Anything, testifymock.Anything).
		Return(nil, nil).Times(2)

	shortLinkTxQ.EXPECT().
		Create(testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(
			_ context.Context,
			id model.ShortLinkID,
			code string,
			destination string,
			_ *time.Time,
			createdAt time.Time,
			updatedAt time.Time,
		) (model.ShortLink, error) {
			return model.ShortLink{
				ID:             id,
				Code:           code,
				DestinationURL: destination,
				CreatedAt:      createdAt,
				UpdatedAt:      updatedAt,
			}, nil
		}).Times(2)

	createdCodes := map[string]struct{}{}
	ids := uuidgen.New()
	reminderTxQ.EXPECT().
		Create(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(_ context.Context, msg repo.CreateRepaymentReminder) (model.RepaymentReminder, error) {
			require.False(t, msg.ID.IsZero())
			_, parseErr := uuid.Parse(msg.ID.String())
			require.NoError(t, parseErr)
			require.False(t, msg.ShortLinkID.IsZero())
			_, parseErr = uuid.Parse(msg.ShortLinkID.String())
			require.NoError(t, parseErr)
			require.Contains(t, msg.PaymentURL, "dpay.ir/")
			code := msg.PaymentURL[len("dpay.ir/"):]
			_, exists := createdCodes[code]
			require.False(t, exists, "each reminder must get a unique short code")
			createdCodes[code] = struct{}{}
			return model.RepaymentReminder{
				ID:          msg.ID,
				CustomerID:  msg.CustomerID,
				ShortLinkID: msg.ShortLinkID,
				PaymentURL:  msg.PaymentURL,
			}, nil
		}).Times(2)

	outboxTxQ.EXPECT().
		Create(testifymock.Anything, testifymock.Anything).
		Return(model.OutboxMessage{}, nil).Times(2)

	destinationPolicy, err := shortlinkdestination.New([]string{"app.example"})
	require.NoError(t, err)

	shortLinkUC, err := shortlinkuc.New(
		pool,
		shortLinkRepo,
		destinationPolicy,
		adapterclock.NewFixed(now, loc),
		ids,
		metricsnoop.New(),
	)
	require.NoError(t, err)

	uc, err := repaymentreminderuc.New(
		pool,
		installment,
		reminder,
		paymentAttempt,
		outbox,
		shortLinkUC,
		adapterclock.NewFixed(now, loc),
		ids,
		metricsnoop.New(),
		"notification.sms.requests",
		"dpay.ir",
		"https://app.example/repayments",
		1,
	)
	require.NoError(t, err)

	out, err := uc.Dispatch(context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, out.CandidateCount)
	require.Equal(t, 2, out.EligibleCustomerCount)
	require.Equal(t, 2, out.CreatedCount)
	require.Equal(t, 0, out.SkippedCount)
	require.Equal(t, 2, len(createdCodes))
	require.GreaterOrEqual(t, listCalls, 2)
}
