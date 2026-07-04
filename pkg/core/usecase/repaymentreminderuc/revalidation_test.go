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

func TestDispatch_OrdinaryRevalidationEdgeCases(t *testing.T) {
	t.Parallel()

	type fixture struct {
		name string
		prep func(h *ordinaryHarness)
	}

	cases := []fixture{
		{
			name: "missing_contract",
			prep: func(h *ordinaryHarness) {
				h.contractTxQ.EXPECT().
					LockByID(testifymock.Anything, h.contractID).
					Return(model.Contract{}, cerr.ResourceNotFound(nil))
			},
		},
		{
			name: "missing_contract_id",
			prep: func(h *ordinaryHarness) {
				row := h.installmentRow
				row.ContractID = nil
				h.installmentRow = row
			},
		},
		{
			name: "customer_ownership_mismatch",
			prep: func(h *ordinaryHarness) {
				h.contractTxQ.EXPECT().
					LockByID(testifymock.Anything, h.contractID).
					Return(model.Contract{
						ID:         h.contractID,
						CustomerID: model.CustomerID(uuid.New()),
						Status:     model.ActiveContractStatus,
					}, nil)
			},
		},
		{
			name: "locked_installment_contract_changed",
			prep: func(h *ordinaryHarness) {
				h.contractTxQ.EXPECT().
					LockByID(testifymock.Anything, h.contractID).
					Return(model.Contract{
						ID:         h.contractID,
						CustomerID: h.customerID,
						Status:     model.ActiveContractStatus,
					}, nil)
				other := model.ContractID(uuid.New())
				locked := h.installmentRow
				locked.ContractID = &other
				h.installmentTxQ.EXPECT().
					LockByID(testifymock.Anything, h.installmentID).
					Return(locked, nil)
			},
		},
		{
			name: "locked_installment_terminal",
			prep: func(h *ordinaryHarness) {
				h.contractTxQ.EXPECT().
					LockByID(testifymock.Anything, h.contractID).
					Return(model.Contract{
						ID:         h.contractID,
						CustomerID: h.customerID,
						Status:     model.ActiveContractStatus,
					}, nil)
				locked := h.installmentRow
				locked.Status = model.PaidInstallmentStatus
				h.installmentTxQ.EXPECT().
					LockByID(testifymock.Anything, h.installmentID).
					Return(locked, nil)
			},
		},
		{
			name: "unresolved_payment_attempt",
			prep: func(h *ordinaryHarness) {
				h.contractTxQ.EXPECT().
					LockByID(testifymock.Anything, h.contractID).
					Return(model.Contract{
						ID:         h.contractID,
						CustomerID: h.customerID,
						Status:     model.ActiveContractStatus,
					}, nil)
				h.installmentTxQ.EXPECT().
					LockByID(testifymock.Anything, h.installmentID).
					Return(h.installmentRow, nil)
				h.paymentAttemptTxQ.EXPECT().
					ListInstallmentIDsWithNonTerminalAttempts(
						testifymock.Anything,
						h.customerID,
						[]model.InstallmentID{h.installmentID},
					).
					Return([]model.InstallmentID{h.installmentID}, nil)
			},
		},
		{
			name: "policy_no_longer_eligible",
			prep: func(h *ordinaryHarness) {
				// Discovery facts still look DueToday, but the locked row's due
				// date has moved so final EvaluateAll no longer matches.
				h.contractTxQ.EXPECT().
					LockByID(testifymock.Anything, h.contractID).
					Return(model.Contract{
						ID:         h.contractID,
						CustomerID: h.customerID,
						Status:     model.ActiveContractStatus,
					}, nil)
				locked := h.installmentRow
				locked.DueAt = h.installmentRow.DueAt.AddDate(0, 0, 30)
				h.installmentTxQ.EXPECT().
					LockByID(testifymock.Anything, h.installmentID).
					Return(locked, nil)
				h.paymentAttemptTxQ.EXPECT().
					ListInstallmentIDsWithNonTerminalAttempts(
						testifymock.Anything,
						h.customerID,
						[]model.InstallmentID{h.installmentID},
					).
					Return(nil, nil)
				h.decisionGetTimes = 2
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newOrdinaryHarness(t)
			tc.prep(h)
			out := h.dispatch(t)
			require.Equal(t, 0, out.CreatedCount, tc.name)
			require.GreaterOrEqual(t, out.SkippedCount, 1, tc.name)
		})
	}
}

func TestDispatch_DefaultRevalidationSuppressesStaleNotice(t *testing.T) {
	t.Parallel()

	t.Run("eligibility_becomes_false", func(t *testing.T) {
		t.Parallel()
		h := newDefaultHarness(t)
		load := h.notice.load
		h.notice = &flippingDefaultNotice{
			load: load,
			persist: notificationv1.Eligibility{
				Eligible:  false,
				EpisodeID: load.EpisodeID,
			},
		}
		out := h.dispatch(t)
		require.Equal(t, 0, out.CreatedCount)
	})

	t.Run("episode_identity_mismatch", func(t *testing.T) {
		t.Parallel()
		h := newDefaultHarness(t)
		load := h.notice.load
		h.notice = &flippingDefaultNotice{
			load: load,
			persist: notificationv1.Eligibility{
				Eligible:       true,
				EpisodeID:      model.DefaultEpisodeID(uuid.New()),
				InstallmentIDs: load.InstallmentIDs,
			},
		}
		out := h.dispatch(t)
		require.Equal(t, 0, out.CreatedCount)
	})

	t.Run("installment_not_in_notice_ids", func(t *testing.T) {
		t.Parallel()
		h := newDefaultHarness(t)
		load := h.notice.load
		h.notice = &flippingDefaultNotice{
			load: load,
			persist: notificationv1.Eligibility{
				Eligible:       true,
				EpisodeID:      load.EpisodeID,
				InstallmentIDs: []model.InstallmentID{model.InstallmentID(uuid.New())},
			},
		}
		out := h.dispatch(t)
		require.Equal(t, 0, out.CreatedCount)
	})

	t.Run("installment_became_terminal", func(t *testing.T) {
		t.Parallel()
		h := newDefaultHarness(t)
		terminal := h.installmentRow
		terminal.Status = model.PaidInstallmentStatus
		h.installmentTxQ.EXPECT().
			LockByID(testifymock.Anything, h.installmentID).
			Return(terminal, nil)
		out := h.dispatch(t)
		require.Equal(t, 0, out.CreatedCount)
	})
}

type ordinaryHarness struct {
	now              time.Time
	customerID       model.CustomerID
	extID            model.ExternalCustomerID
	installmentID    model.InstallmentID
	contractID       model.ContractID
	decisionID       model.DecisionID
	installmentRow   model.Installment
	decisionGetTimes int

	pool              *mock.Pool
	conn              *mock.Conn
	tx                *mock.Tx
	installment       *mock.Installment
	installmentConnQ  *mock.InstallmentConnQueryer
	installmentTxQ    *mock.InstallmentTxQueryer
	contract          *mock.Contract
	contractTxQ       *mock.ContractTxQueryer
	reminder          *mock.RepaymentReminder
	reminderTxQ       *mock.RepaymentReminderTxQueryer
	paymentAttempt    *mock.PaymentAttempt
	paymentAttemptTxQ *mock.PaymentAttemptTxQueryer
	decisionRepo      *mock.Decision
	decisionTxQ       *mock.DecisionTxQueryer
	outbox            *mock.Outbox
	outboxTxQ         *mock.OutboxTxQueryer
	shortLinkRepo     *mock.ShortLink
	shortLinkTxQ      *mock.ShortLinkTxQueryer
	checkoutPolicies  *checkout.Registry
}

func newOrdinaryHarness(t *testing.T) *ordinaryHarness {
	t.Helper()

	loc := time.UTC
	now := time.Date(2026, time.March, 10, 10, 0, 0, 0, loc)
	businessDate := time.Date(2026, time.March, 10, 0, 0, 0, 0, loc)
	customerID := model.CustomerID(uuid.New())
	contractID := model.ContractID(uuid.New())
	installmentID := model.InstallmentID(uuid.New())
	decisionID := model.DecisionID(uuid.New())

	h := &ordinaryHarness{
		now:           now,
		customerID:    customerID,
		extID:         model.ExternalCustomerID(uuid.New()),
		installmentID: installmentID,
		contractID:    contractID,
		decisionID:    decisionID,
		installmentRow: model.Installment{
			ID:              installmentID,
			DecisionID:      decisionID,
			ContractID:      &contractID,
			CustomerID:      customerID,
			DueAt:           businessDate.UTC(),
			GracePeriodDays: 5,
			Status:          model.DueInstallmentStatus,
		},
		decisionGetTimes:  1,
		pool:              mock.NewPool(t),
		conn:              mock.NewConn(t),
		tx:                mock.NewTx(t),
		installment:       mock.NewInstallment(t),
		installmentConnQ:  mock.NewInstallmentConnQueryer(t),
		installmentTxQ:    mock.NewInstallmentTxQueryer(t),
		contract:          mock.NewContract(t),
		contractTxQ:       mock.NewContractTxQueryer(t),
		reminder:          mock.NewRepaymentReminder(t),
		reminderTxQ:       mock.NewRepaymentReminderTxQueryer(t),
		paymentAttempt:    mock.NewPaymentAttempt(t),
		paymentAttemptTxQ: mock.NewPaymentAttemptTxQueryer(t),
		decisionRepo:      mock.NewDecision(t),
		decisionTxQ:       mock.NewDecisionTxQueryer(t),
		outbox:            mock.NewOutbox(t),
		outboxTxQ:         mock.NewOutboxTxQueryer(t),
		shortLinkRepo:     mock.NewShortLink(t),
		shortLinkTxQ:      mock.NewShortLinkTxQueryer(t),
	}

	var err error
	h.checkoutPolicies, err = checkout.NewRegistry(checkoutv1.New())
	require.NoError(t, err)
	return h
}

func (h *ordinaryHarness) dispatch(t *testing.T) repaymentreminderuc.DispatchOutput {
	t.Helper()

	v1Ref := checkoutv1.New().Ref()
	var listCalls int

	h.pool.EXPECT().Conn(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(ctx context.Context, fn repo.ConnHandler) error {
			return fn(ctx, h.conn)
		})
	h.conn.EXPECT().Tx(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(ctx context.Context, fn repo.TxHandler) error {
			return fn(ctx, h.tx)
		}).Maybe()

	h.installment.EXPECT().Conn(h.conn).Return(h.installmentConnQ)
	h.installmentConnQ.EXPECT().
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
					Installment:        h.installmentRow,
					ExternalCustomerID: h.extID,
				}}, nil
			}

			return nil, nil
		})

	h.installment.EXPECT().Tx(h.tx).Return(h.installmentTxQ).Maybe()
	h.contract.EXPECT().Tx(h.tx).Return(h.contractTxQ).Maybe()
	h.reminder.EXPECT().Tx(h.tx).Return(h.reminderTxQ)
	h.paymentAttempt.EXPECT().Tx(h.tx).Return(h.paymentAttemptTxQ).Maybe()
	h.decisionRepo.EXPECT().Tx(h.tx).Return(h.decisionTxQ).Maybe()
	h.outbox.EXPECT().Tx(h.tx).Return(h.outboxTxQ).Maybe()
	h.shortLinkRepo.EXPECT().Tx(h.tx).Return(h.shortLinkTxQ).Maybe()

	h.decisionTxQ.EXPECT().
		GetByID(testifymock.Anything, h.decisionID).
		Return(model.Decision{ID: h.decisionID, PolicyID: v1Ref.ID, PolicyVersion: v1Ref.Version}, nil).
		Times(h.decisionGetTimes).
		Maybe()

	if h.installmentRow.ContractID != nil {
		h.installmentTxQ.EXPECT().
			GetByID(testifymock.Anything, h.installmentID).
			Return(h.installmentRow, nil).
			Maybe()
	} else {
		h.installmentTxQ.EXPECT().
			GetByID(testifymock.Anything, h.installmentID).
			Return(h.installmentRow, nil)
	}

	h.reminderTxQ.EXPECT().
		GetByNotificationIdempotencyKey(testifymock.Anything, testifymock.Anything).
		Return(model.RepaymentReminder{}, cerr.ResourceNotFound(nil)).
		Maybe()

	destinationPolicy, err := shortlinkdestination.New([]string{"app.example"})
	require.NoError(t, err)

	ids := uuidgen.New()
	shortLinkUC, err := shortlinkuc.New(
		h.pool,
		h.shortLinkRepo,
		destinationPolicy,
		adapterclock.NewFixed(h.now, time.UTC),
		ids,
		metricsnoop.New(),
	)
	require.NoError(t, err)

	uc, err := repaymentreminderuc.New(
		h.pool,
		h.installment,
		h.contract,
		h.reminder,
		h.paymentAttempt,
		h.decisionRepo,
		h.checkoutPolicies,
		h.outbox,
		shortLinkUC,
		adapterclock.NewFixed(h.now, time.UTC),
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

type flippingDefaultNotice struct {
	calls   int
	load    notificationv1.Eligibility
	persist notificationv1.Eligibility
}

func (s *flippingDefaultNotice) DefaultNoticeOnTx(
	context.Context,
	repo.Tx,
	model.CustomerID,
	time.Time,
) (notificationv1.Eligibility, error) {
	s.calls++
	if s.calls == 1 {
		return s.load, nil
	}

	return s.persist, nil
}

type defaultHarness struct {
	customerID     model.CustomerID
	extID          model.ExternalCustomerID
	installmentID  model.InstallmentID
	contractID     model.ContractID
	decisionID     model.DecisionID
	episodeID      model.DefaultEpisodeID
	installmentRow model.Installment
	notice         *flippingDefaultNotice

	pool             *mock.Pool
	conn             *mock.Conn
	tx               *mock.Tx
	installment      *mock.Installment
	installmentConnQ *mock.InstallmentConnQueryer
	installmentTxQ   *mock.InstallmentTxQueryer
	contract         *mock.Contract
	contractTxQ      *mock.ContractTxQueryer
	reminder         *mock.RepaymentReminder
	reminderTxQ      *mock.RepaymentReminderTxQueryer
	paymentAttempt   *mock.PaymentAttempt
	decisionRepo     *mock.Decision
	decisionTxQ      *mock.DecisionTxQueryer
	outbox           *mock.Outbox
	outboxTxQ        *mock.OutboxTxQueryer
	shortLinkRepo    *mock.ShortLink
	shortLinkTxQ     *mock.ShortLinkTxQueryer
	checkoutPolicies *checkout.Registry
}

func newDefaultHarness(t *testing.T) *defaultHarness {
	t.Helper()

	loc := time.UTC
	businessDate := time.Date(2026, time.March, 10, 0, 0, 0, 0, loc)
	customerID := model.CustomerID(uuid.New())
	contractID := model.ContractID(uuid.New())
	installmentID := model.InstallmentID(uuid.New())
	decisionID := model.DecisionID(uuid.New())
	episodeID := model.DefaultEpisodeID(uuid.New())

	h := &defaultHarness{
		customerID:    customerID,
		extID:         model.ExternalCustomerID(uuid.New()),
		installmentID: installmentID,
		contractID:    contractID,
		decisionID:    decisionID,
		episodeID:     episodeID,
		installmentRow: model.Installment{
			ID:              installmentID,
			DecisionID:      decisionID,
			ContractID:      &contractID,
			CustomerID:      customerID,
			DueAt:           businessDate.AddDate(0, 0, -10).UTC(),
			GracePeriodDays: 5,
			Status:          model.OverdueInstallmentStatus,
		},
		notice: &flippingDefaultNotice{
			load: notificationv1.Eligibility{
				Eligible:       true,
				EpisodeID:      episodeID,
				InstallmentIDs: []model.InstallmentID{installmentID},
			},
			persist: notificationv1.Eligibility{
				Eligible:       true,
				EpisodeID:      episodeID,
				InstallmentIDs: []model.InstallmentID{installmentID},
			},
		},
		pool:             mock.NewPool(t),
		conn:             mock.NewConn(t),
		tx:               mock.NewTx(t),
		installment:      mock.NewInstallment(t),
		installmentConnQ: mock.NewInstallmentConnQueryer(t),
		installmentTxQ:   mock.NewInstallmentTxQueryer(t),
		contract:         mock.NewContract(t),
		contractTxQ:      mock.NewContractTxQueryer(t),
		reminder:         mock.NewRepaymentReminder(t),
		reminderTxQ:      mock.NewRepaymentReminderTxQueryer(t),
		paymentAttempt:   mock.NewPaymentAttempt(t),
		decisionRepo:     mock.NewDecision(t),
		decisionTxQ:      mock.NewDecisionTxQueryer(t),
		outbox:           mock.NewOutbox(t),
		outboxTxQ:        mock.NewOutboxTxQueryer(t),
		shortLinkRepo:    mock.NewShortLink(t),
		shortLinkTxQ:     mock.NewShortLinkTxQueryer(t),
	}

	var err error
	h.checkoutPolicies, err = checkout.NewRegistry(checkoutv1.New())
	require.NoError(t, err)

	// Wire common discovery / persist scaffolding. Specific LockByID expectations
	// are left to each subtest so stale Default revalidation stays explicit.
	var listCalls int
	h.pool.EXPECT().Conn(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(ctx context.Context, fn repo.ConnHandler) error {
			return fn(ctx, h.conn)
		})
	h.conn.EXPECT().Tx(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(ctx context.Context, fn repo.TxHandler) error {
			return fn(ctx, h.tx)
		})

	h.installment.EXPECT().Conn(h.conn).Return(h.installmentConnQ)
	h.installmentConnQ.EXPECT().
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
					Installment:        h.installmentRow,
					ExternalCustomerID: h.extID,
				}}, nil
			}

			return nil, nil
		})

	h.installment.EXPECT().Tx(h.tx).Return(h.installmentTxQ).Maybe()
	h.contract.EXPECT().Tx(h.tx).Return(h.contractTxQ).Maybe()
	h.reminder.EXPECT().Tx(h.tx).Return(h.reminderTxQ)
	h.paymentAttempt.EXPECT().Tx(h.tx).Return(mock.NewPaymentAttemptTxQueryer(t)).Maybe()
	h.decisionRepo.EXPECT().Tx(h.tx).Return(h.decisionTxQ).Maybe()
	h.outbox.EXPECT().Tx(h.tx).Return(h.outboxTxQ).Maybe()
	h.shortLinkRepo.EXPECT().Tx(h.tx).Return(h.shortLinkTxQ).Maybe()

	v1Ref := checkoutv1.New().Ref()
	h.decisionTxQ.EXPECT().
		GetByID(testifymock.Anything, h.decisionID).
		Return(model.Decision{ID: h.decisionID, PolicyID: v1Ref.ID, PolicyVersion: v1Ref.Version}, nil).
		Maybe()

	h.reminderTxQ.EXPECT().
		GetByNotificationIdempotencyKey(testifymock.Anything, testifymock.Anything).
		Return(model.RepaymentReminder{}, cerr.ResourceNotFound(nil)).
		Maybe()

	return h
}

func (h *defaultHarness) dispatch(t *testing.T) repaymentreminderuc.DispatchOutput {
	t.Helper()

	// Default path locks the installment unless a subtest already set a
	// more specific expectation.
	h.installmentTxQ.EXPECT().
		LockByID(testifymock.Anything, h.installmentID).
		Return(h.installmentRow, nil).
		Maybe()

	destinationPolicy, err := shortlinkdestination.New([]string{"app.example"})
	require.NoError(t, err)

	ids := uuidgen.New()
	now := time.Date(2026, time.March, 10, 10, 0, 0, 0, time.UTC)
	shortLinkUC, err := shortlinkuc.New(
		h.pool,
		h.shortLinkRepo,
		destinationPolicy,
		adapterclock.NewFixed(now, time.UTC),
		ids,
		metricsnoop.New(),
	)
	require.NoError(t, err)

	uc, err := repaymentreminderuc.New(
		h.pool,
		h.installment,
		h.contract,
		h.reminder,
		h.paymentAttempt,
		h.decisionRepo,
		h.checkoutPolicies,
		h.outbox,
		shortLinkUC,
		adapterclock.NewFixed(now, time.UTC),
		ids,
		metricsnoop.New(),
		"notification.sms.requests",
		"dpay.ir",
		"https://app.example/repayments",
		10,
		1,
	)
	require.NoError(t, err)
	uc.WithDefaultNotifications(h.notice)

	out, err := uc.Dispatch(context.Background())
	require.NoError(t, err)
	return out
}
