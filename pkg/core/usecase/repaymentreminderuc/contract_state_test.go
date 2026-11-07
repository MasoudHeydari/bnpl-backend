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
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
	checkoutv1 "github.com/parstis/bnpl-backend/pkg/core/policy/checkout/v1"
	notificationv1 "github.com/parstis/bnpl-backend/pkg/core/policy/defaultnotification/v1"
	"github.com/parstis/bnpl-backend/pkg/core/policy/shortlinkdestination"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/repaymentreminderuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/shortlinkuc"
)

type stubDefaultNotice struct {
	eligibility notificationv1.Eligibility
}

func (s stubDefaultNotice) DefaultNoticeOnTx(
	context.Context,
	repo.Tx,
	model.CustomerID,
	time.Time,
) (notificationv1.Eligibility, error) {
	return s.eligibility, nil
}

func TestDispatch_SuppressesOrdinaryReminderWhenContractNotActive(t *testing.T) {
	t.Parallel()

	statuses := []model.ContractStatus{
		model.CancelingContractStatus,
		model.PaymentPendingContractStatus,
		model.CompletedContractStatus,
		model.CanceledContractStatus,
		model.ExpiredContractStatus,
		model.DefaultedContractStatus,
	}

	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()

			out := dispatchOrdinaryReminder(t, status, false)
			require.Equal(t, 1, out.CandidateCount)
			require.Equal(t, 0, out.CreatedCount)
			require.Equal(t, 1, out.SkippedCount)
		})
	}
}

func TestDispatch_PersistsOrdinaryReminderWhenContractActive(t *testing.T) {
	t.Parallel()

	out := dispatchOrdinaryReminder(t, model.ActiveContractStatus, true)
	require.Equal(t, 1, out.CandidateCount)
	require.Equal(t, 1, out.CreatedCount)
	require.Equal(t, 0, out.SkippedCount)
}

func TestDispatch_DefaultNoticeIndependentOfCancelingContract(t *testing.T) {
	t.Parallel()

	loc := time.UTC
	now := time.Date(2026, time.March, 10, 10, 0, 0, 0, loc)
	businessDate := time.Date(2026, time.March, 10, 0, 0, 0, 0, loc)
	customerID := model.CustomerID(uuid.New())
	extID := model.ExternalCustomerID(uuid.New())
	installmentID := model.InstallmentID(uuid.New())
	contractID := model.ContractID(uuid.New())
	decisionID := model.DecisionID(uuid.New())
	episodeID := model.DefaultEpisodeID(uuid.New())
	v1Ref := checkoutv1.New().Ref()
	dueToday := businessDate.UTC()

	installmentRow := model.Installment{
		ID:              installmentID,
		DecisionID:      decisionID,
		ContractID:      &contractID,
		CustomerID:      customerID,
		DueAt:           dueToday,
		GracePeriodDays: 5,
		Status:          model.OverdueInstallmentStatus,
	}

	pool := mock.NewPool(t)
	conn := mock.NewConn(t)
	tx := mock.NewTx(t)
	installment := mock.NewInstallment(t)
	installmentConnQ := mock.NewInstallmentConnQueryer(t)
	installmentTxQ := mock.NewInstallmentTxQueryer(t)
	contract := mock.NewContract(t)
	contractTxQ := mock.NewContractTxQueryer(t)
	reminder := mock.NewRepaymentReminder(t)
	reminderTxQ := mock.NewRepaymentReminderTxQueryer(t)
	paymentAttempt := mock.NewPaymentAttempt(t)
	paymentAttemptTxQ := mock.NewPaymentAttemptTxQueryer(t)
	decisionRepo := mock.NewDecision(t)
	decisionTxQ := mock.NewDecisionTxQueryer(t)
	outbox := mock.NewOutbox(t)
	outboxTxQ := mock.NewOutboxTxQueryer(t)
	shortLinkRepo := mock.NewShortLink(t)
	shortLinkTxQ := mock.NewShortLinkTxQueryer(t)
	checkoutPolicies, err := checkout.NewRegistry(checkoutv1.New())
	require.NoError(t, err)

	var listCalls int
	pool.EXPECT().Conn(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(ctx context.Context, fn repo.ConnHandler) error {
			return fn(ctx, conn)
		})
	conn.EXPECT().Tx(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(ctx context.Context, fn repo.TxHandler) error {
			return fn(ctx, tx)
		})

	installment.EXPECT().Conn(conn).Return(installmentConnQ)
	installmentConnQ.EXPECT().
		ListReminderCandidatePage(
			testifymock.Anything,
			testifymock.Anything,
			testifymock.Anything,
			testifymock.Anything,
			testifymock.Anything,
			testifymock.Anything,
			"UTC",
		).
		RunAndReturn(func(
			context.Context,
			time.Time,
			time.Time,
			time.Time,
			model.CustomerID,
			int,
			string,
		) ([]repo.ReminderCandidate, error) {
			listCalls++
			if listCalls == 1 {
				return []repo.ReminderCandidate{{
					Installment:        installmentRow,
					ExternalCustomerID: extID,
				}}, nil
			}

			return nil, nil
		})

	installment.EXPECT().Tx(tx).Return(installmentTxQ).Maybe()
	contract.EXPECT().Tx(tx).Return(contractTxQ).Maybe()
	reminder.EXPECT().Tx(tx).Return(reminderTxQ)
	paymentAttempt.EXPECT().Tx(tx).Return(paymentAttemptTxQ).Maybe()
	decisionRepo.EXPECT().Tx(tx).Return(decisionTxQ)
	outbox.EXPECT().Tx(tx).Return(outboxTxQ)
	shortLinkRepo.EXPECT().Tx(tx).Return(shortLinkTxQ)

	decisionTxQ.EXPECT().
		GetByID(testifymock.Anything, decisionID).
		Return(model.Decision{ID: decisionID, PolicyID: v1Ref.ID, PolicyVersion: v1Ref.Version}, nil).
		Maybe()

	// Ordinary path: canceling contract suppresses DueToday without locking the
	// installment after Contract.LockByID returns a non-active status.
	installmentTxQ.EXPECT().
		GetByID(testifymock.Anything, installmentID).
		Return(installmentRow, nil).
		Once()
	contractTxQ.EXPECT().
		LockByID(testifymock.Anything, contractID).
		Return(model.Contract{
			ID:         contractID,
			CustomerID: customerID,
			Status:     model.CancelingContractStatus,
		}, nil).
		Once()

	// Default path: customer lock is inside stub; installment lock only.
	installmentTxQ.EXPECT().
		LockByID(testifymock.Anything, installmentID).
		Return(installmentRow, nil).
		Once()

	reminderTxQ.EXPECT().
		GetByNotificationIdempotencyKey(testifymock.Anything, testifymock.Anything).
		Return(model.RepaymentReminder{}, cerr.ResourceNotFound(nil)).
		Times(2)

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
		}).Once()

	var createdTypes []model.ReminderType
	ids := uuidgen.New()
	reminderTxQ.EXPECT().
		Create(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(_ context.Context, msg repo.CreateRepaymentReminder) (model.RepaymentReminder, error) {
			createdTypes = append(createdTypes, msg.ReminderType)
			return model.RepaymentReminder{ID: msg.ID, CustomerID: msg.CustomerID}, nil
		}).Once()

	outboxTxQ.EXPECT().
		Create(testifymock.Anything, testifymock.Anything).
		Return(model.OutboxMessage{}, nil).Once()

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
		contract,
		reminder,
		paymentAttempt,
		decisionRepo,
		checkoutPolicies,
		outbox,
		shortLinkUC,
		adapterclock.NewFixed(now, loc),
		ids,
		metricsnoop.New(),
		"notification.sms.requests",
		"dpay.ir",
		"https://app.example/repayments",
		10,
		1,
	)
	require.NoError(t, err)
	uc.WithDefaultNotifications(stubDefaultNotice{
		eligibility: notificationv1.Eligibility{
			Eligible:       true,
			EpisodeID:      episodeID,
			InstallmentIDs: []model.InstallmentID{installmentID},
		},
	})

	out, err := uc.Dispatch(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, out.CreatedCount)
	require.Equal(t, []model.ReminderType{model.DefaultReminderType}, createdTypes)
}

func dispatchOrdinaryReminder(
	t *testing.T,
	status model.ContractStatus,
	expectCreate bool,
) repaymentreminderuc.DispatchOutput {
	t.Helper()

	loc := time.UTC
	now := time.Date(2026, time.March, 10, 10, 0, 0, 0, loc)
	businessDate := time.Date(2026, time.March, 10, 0, 0, 0, 0, loc)
	customerID := model.CustomerID(uuid.New())
	extID := model.ExternalCustomerID(uuid.New())
	installmentID := model.InstallmentID(uuid.New())
	contractID := model.ContractID(uuid.New())
	decisionID := model.DecisionID(uuid.New())
	v1Ref := checkoutv1.New().Ref()
	dueToday := businessDate.UTC()

	installmentRow := model.Installment{
		ID:              installmentID,
		DecisionID:      decisionID,
		ContractID:      &contractID,
		CustomerID:      customerID,
		DueAt:           dueToday,
		GracePeriodDays: 5,
		Status:          model.DueInstallmentStatus,
	}

	pool := mock.NewPool(t)
	conn := mock.NewConn(t)
	tx := mock.NewTx(t)
	installment := mock.NewInstallment(t)
	installmentConnQ := mock.NewInstallmentConnQueryer(t)
	installmentTxQ := mock.NewInstallmentTxQueryer(t)
	contract := mock.NewContract(t)
	contractTxQ := mock.NewContractTxQueryer(t)
	reminder := mock.NewRepaymentReminder(t)
	reminderTxQ := mock.NewRepaymentReminderTxQueryer(t)
	paymentAttempt := mock.NewPaymentAttempt(t)
	paymentAttemptTxQ := mock.NewPaymentAttemptTxQueryer(t)
	decisionRepo := mock.NewDecision(t)
	decisionTxQ := mock.NewDecisionTxQueryer(t)
	outbox := mock.NewOutbox(t)
	outboxTxQ := mock.NewOutboxTxQueryer(t)
	shortLinkRepo := mock.NewShortLink(t)
	shortLinkTxQ := mock.NewShortLinkTxQueryer(t)
	checkoutPolicies, err := checkout.NewRegistry(checkoutv1.New())
	require.NoError(t, err)

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
			testifymock.Anything,
			testifymock.Anything,
			testifymock.Anything,
			"UTC",
		).
		RunAndReturn(func(
			context.Context,
			time.Time,
			time.Time,
			time.Time,
			model.CustomerID,
			int,
			string,
		) ([]repo.ReminderCandidate, error) {
			listCalls++
			if listCalls == 1 {
				return []repo.ReminderCandidate{{
					Installment:        installmentRow,
					ExternalCustomerID: extID,
				}}, nil
			}

			return nil, nil
		})

	installment.EXPECT().Tx(tx).Return(installmentTxQ)
	contract.EXPECT().Tx(tx).Return(contractTxQ)
	reminder.EXPECT().Tx(tx).Return(reminderTxQ)
	paymentAttempt.EXPECT().Tx(tx).Return(paymentAttemptTxQ).Maybe()
	decisionRepo.EXPECT().Tx(tx).Return(decisionTxQ)
	outbox.EXPECT().Tx(tx).Return(outboxTxQ).Maybe()
	shortLinkRepo.EXPECT().Tx(tx).Return(shortLinkTxQ).Maybe()

	decisionGetTimes := 1
	if expectCreate {
		decisionGetTimes = 2
	}

	decisionTxQ.EXPECT().
		GetByID(testifymock.Anything, decisionID).
		Return(model.Decision{ID: decisionID, PolicyID: v1Ref.ID, PolicyVersion: v1Ref.Version}, nil).
		Times(decisionGetTimes)

	installmentTxQ.EXPECT().
		GetByID(testifymock.Anything, installmentID).
		Return(installmentRow, nil)
	contractTxQ.EXPECT().
		LockByID(testifymock.Anything, contractID).
		Return(model.Contract{
			ID:         contractID,
			CustomerID: customerID,
			Status:     status,
		}, nil)

	reminderTxQ.EXPECT().
		GetByNotificationIdempotencyKey(testifymock.Anything, testifymock.Anything).
		Return(model.RepaymentReminder{}, cerr.ResourceNotFound(nil))

	if expectCreate {
		installmentTxQ.EXPECT().
			LockByID(testifymock.Anything, installmentID).
			Return(installmentRow, nil)
		paymentAttemptTxQ.EXPECT().
			ListInstallmentIDsWithNonTerminalAttempts(testifymock.Anything, customerID, []model.InstallmentID{installmentID}).
			Return(nil, nil)
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
			})
		reminderTxQ.EXPECT().
			Create(testifymock.Anything, testifymock.Anything).
			RunAndReturn(func(_ context.Context, msg repo.CreateRepaymentReminder) (model.RepaymentReminder, error) {
				return model.RepaymentReminder{ID: msg.ID, CustomerID: msg.CustomerID}, nil
			})
		outboxTxQ.EXPECT().
			Create(testifymock.Anything, testifymock.Anything).
			Return(model.OutboxMessage{}, nil)
	}

	destinationPolicy, err := shortlinkdestination.New([]string{"app.example"})
	require.NoError(t, err)

	ids := uuidgen.New()
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
		contract,
		reminder,
		paymentAttempt,
		decisionRepo,
		checkoutPolicies,
		outbox,
		shortLinkUC,
		adapterclock.NewFixed(now, loc),
		ids,
		metricsnoop.New(),
		"notification.sms.requests",
		"dpay.ir",
		"https://app.example/repayments",
		10,
		1,
	)
	require.NoError(t, err)

	out, err := uc.Dispatch(context.Background())
	require.NoError(t, err)
	return out
}
