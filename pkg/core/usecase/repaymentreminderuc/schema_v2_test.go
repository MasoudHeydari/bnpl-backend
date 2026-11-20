package repaymentreminderuc_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	adapterclock "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/identifier/uuidgen"
	metricsnoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/metrics/noop"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/event/notificationsms"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
	checkoutv1 "github.com/parstis/bnpl-backend/pkg/core/policy/checkout/v1"
	defaultnotificationv1 "github.com/parstis/bnpl-backend/pkg/core/policy/defaultnotification/v1"
	reminderv1 "github.com/parstis/bnpl-backend/pkg/core/policy/repaymentreminder/v1"
	"github.com/parstis/bnpl-backend/pkg/core/policy/shortlinkdestination"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/repaymentreminderuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/shortlinkuc"
)

func TestDispatch_NotificationCommandPayload(t *testing.T) {
	t.Parallel()

	payload, eventType := dispatchAndCapturePayload(t, context.Background())
	require.Equal(t, notificationsms.EventTypeCommand, eventType)

	var event notificationsms.Command
	require.NoError(t, json.Unmarshal(payload, &event))
	require.NoError(t, notificationsms.ValidateCommand(event))
	require.Equal(t, notificationsms.SchemaVersionCommand, event.SchemaVersion)
	require.Equal(t, notificationsms.ProducerName, event.SourceService)
	require.Equal(t, notificationsms.DefaultPriority, event.Priority)
	require.Equal(t, reminderv1.TemplateDueToday, event.NotificationType)
	require.Equal(t, reminderv1.TemplateDueToday, event.Targets[0].Deliveries[0].TemplateCode)
	require.Equal(t, notificationsms.SelectionSourceUpstreamSelected, event.Targets[0].Deliveries[0].SelectionSource)
	require.Equal(t, notificationsms.BusinessReferenceTypeBNPLInstallment, event.Targets[0].BusinessReferenceType)
	require.True(t, strings.HasPrefix(event.Targets[0].Deliveries[0].Variables["payment_url"], "https://"))
	require.NotContains(t, string(payload), `"recipient_address"`)
	require.NotContains(t, string(payload), `"recipient_hash"`)
	require.NotContains(t, string(payload), `"recipient"`)
	require.NotContains(t, string(payload), `"correlation_id"`)
	require.NotContains(t, string(payload), `"business_scope"`)

	loc := time.UTC
	businessDate := time.Date(2026, time.March, 10, 0, 0, 0, 0, loc)
	require.True(t, event.ExpiresAt.Equal(
		notificationsms.DeliveryDeadlineForBusinessDate(businessDate),
	))
	require.Contains(t, event.IdempotencyKey, ":due_today:2026-03-10")
}

func TestDispatch_Command_IndependentBusinessDatesAndDefaultKey(t *testing.T) {
	t.Parallel()

	installmentID := model.InstallmentID(uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"))
	dayA := time.Date(2026, time.November, 7, 0, 0, 0, 0, time.UTC)
	dayB := time.Date(2026, time.November, 8, 0, 0, 0, 0, time.UTC)

	keyA := reminderv1.NotificationIdempotencyKey(installmentID, model.PreDueReminderType, dayA)
	keyB := reminderv1.NotificationIdempotencyKey(installmentID, model.PreDueReminderType, dayB)
	require.NotEqual(t, keyA, keyB)
	require.Equal(t, "repayment:bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb:pre_due:2026-11-07", keyA)
	require.Equal(t, "repayment:bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb:pre_due:2026-11-08", keyB)

	episodeID := model.DefaultEpisodeID(uuid.MustParse("cccccccc-cccc-cccc-cccc-cccccccccccc"))
	defaultKey := reminderv1.DefaultNotificationIdempotencyKey(episodeID, installmentID)
	require.Equal(t, "default:cccccccc-cccc-cccc-cccc-cccccccccccc:bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", defaultKey)

	event := notificationsms.NewCommand(notificationsms.CommandInput{
		SourceEventID:      uuid.NewString(),
		IdempotencyKey:     defaultKey,
		NotificationType:   defaultnotificationv1.TemplateID,
		ExternalCustomerID: uuid.NewString(),
		InstallmentID:      installmentID.String(),
		PaymentURL:         "https://dpay.ir/default",
		ExpiresAt:          notificationsms.DeliveryDeadlineForBusinessDate(dayA),
	})
	require.NoError(t, notificationsms.ValidateCommand(event))
	require.Equal(t, defaultKey, event.IdempotencyKey)
}

func dispatchAndCapturePayload(
	t *testing.T,
	ctx context.Context,
) ([]byte, string) {
	t.Helper()

	loc := time.UTC
	now := time.Date(2026, time.March, 10, 10, 0, 0, 0, loc)
	businessDate := time.Date(2026, time.March, 10, 0, 0, 0, 0, loc)

	customerID := model.CustomerID(uuid.MustParse("00000000-0000-0000-0000-0000000000aa"))
	extCustomerID := model.ExternalCustomerID(uuid.MustParse("00000000-0000-0000-0000-0000000000ee"))
	installmentID := model.InstallmentID(uuid.MustParse("00000000-0000-0000-0000-000000000011"))
	contractID := model.ContractID(uuid.New())
	decisionID := model.DecisionID(uuid.New())
	v1Ref := checkoutv1.New().Ref()

	page := []repo.ReminderCandidate{{
		Installment: model.Installment{
			ID:              installmentID,
			DecisionID:      decisionID,
			ContractID:      &contractID,
			CustomerID:      customerID,
			DueAt:           businessDate.UTC(),
			GracePeriodDays: 5,
			Status:          model.DueInstallmentStatus,
		},
		ExternalCustomerID: extCustomerID,
	}}

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
			10,
			"UTC",
		).
		Return(page, nil).Once()
	installmentConnQ.EXPECT().
		ListReminderCandidatePage(
			testifymock.Anything,
			testifymock.Anything,
			testifymock.Anything,
			testifymock.Anything,
			testifymock.Anything,
			10,
			"UTC",
		).
		Return(nil, nil).Maybe()

	installment.EXPECT().Tx(tx).Return(installmentTxQ)
	contract.EXPECT().Tx(tx).Return(contractTxQ)
	reminder.EXPECT().Tx(tx).Return(reminderTxQ)
	paymentAttempt.EXPECT().Tx(tx).Return(paymentAttemptTxQ)
	decisionRepo.EXPECT().Tx(tx).Return(decisionTxQ)
	outbox.EXPECT().Tx(tx).Return(outboxTxQ)
	shortLinkRepo.EXPECT().Tx(tx).Return(shortLinkTxQ)

	decisionTxQ.EXPECT().
		GetByID(testifymock.Anything, decisionID).
		Return(model.Decision{ID: decisionID, PolicyID: v1Ref.ID, PolicyVersion: v1Ref.Version}, nil).
		Times(2)

	installmentTxQ.EXPECT().
		GetByID(testifymock.Anything, installmentID).
		Return(page[0].Installment, nil)

	contractTxQ.EXPECT().
		LockByID(testifymock.Anything, contractID).
		Return(model.Contract{
			ID:         contractID,
			CustomerID: customerID,
			Status:     model.ActiveContractStatus,
		}, nil)

	installmentTxQ.EXPECT().
		LockByID(testifymock.Anything, installmentID).
		Return(page[0].Installment, nil)

	paymentAttemptTxQ.EXPECT().
		ListInstallmentIDsWithNonTerminalAttempts(testifymock.Anything, testifymock.Anything, testifymock.Anything).
		Return(nil, nil)

	reminderTxQ.EXPECT().
		GetByNotificationIdempotencyKey(testifymock.Anything, testifymock.Anything).
		Return(model.RepaymentReminder{}, cerr.ResourceNotFound(nil))

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
			return model.RepaymentReminder{
				ID:          msg.ID,
				CustomerID:  msg.CustomerID,
				ShortLinkID: msg.ShortLinkID,
				PaymentURL:  msg.PaymentURL,
			}, nil
		})

	var captured repo.CreateOutboxMessage
	outboxTxQ.EXPECT().
		Create(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(_ context.Context, msg repo.CreateOutboxMessage) (model.OutboxMessage, error) {
			captured = msg
			return model.OutboxMessage{}, nil
		})

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
		"notification.commands.v1",
		"dpay.ir",
		"https://app.example/repayments",
		10,
	)
	require.NoError(t, err)

	out, err := uc.Dispatch(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, out.CreatedCount)
	require.NotEmpty(t, captured.Payload)
	require.Equal(t, extCustomerID.String(), captured.MessageKey)

	return captured.Payload, captured.EventType
}
