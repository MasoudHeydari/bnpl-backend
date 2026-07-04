package financialcorepaymentresultuc

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	metricsnoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/metrics/noop"

	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/event/paymentapplication"
	"github.com/parstis/bnpl-backend/pkg/core/identifier"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/contractuc"
)

func TestInsertPaymentApplicationOutbox_UsesConfiguredDestination(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(
		2026,
		7,
		8,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	outboxQ := mock.NewOutboxTxQueryer(t)

	uc := &UseCase{
		paymentApplicationResultsDestination: "financial_core.payment_application_results",
		idGenerator:                          testIDGenerator{value: "outbox-event-1"},
	}

	paidAmount := uint64(1000)
	currency := model.IRRCurrency

	event := paymentapplication.NewEvent(
		"outbox-event-1",
		"evt-1",
		nil,
		now,
		paymentapplication.EventData{
			FinancialPaymentIntentID: "pi_123",
			PaymentPurpose:           model.InstallmentRepaymentFinancialCorePaymentPurpose,
			Result:                   paymentapplication.AppliedApplicationResult,
			PaidAmountMinor:          &paidAmount,
			Currency:                 &currency,
			FinancialPaidAt:          &now,
		},
	)

	outboxQ.EXPECT().Create(
		ctx,
		testifymock.MatchedBy(func(message repo.CreateOutboxMessage) bool {
			require.Equal(t, "financial_core.payment_application_results", message.Destination)
			require.Equal(t, "pi_123", message.MessageKey)
			require.Equal(t, paymentapplication.PaymentApplicationResultEventType, message.EventType)

			var decoded paymentapplication.Event
			require.NoError(t, json.Unmarshal(message.Payload, &decoded))
			require.Equal(t, event.EventID, decoded.EventID)
			require.Equal(t, paymentapplication.PaymentApplicationResultEventType, decoded.EventType)
			require.Equal(t, paymentapplication.PaymentApplicationResultEventVersion, decoded.EventVersion)
			require.Equal(t, paymentapplication.ProducerName, decoded.Producer)
			require.Equal(t, paymentapplication.AppliedApplicationResult, decoded.Data.Result)

			return true
		}),
	).Return(model.OutboxMessage{}, nil).Once()

	err := uc.insertPaymentApplicationOutbox(ctx, outboxQ, event, now)
	require.NoError(t, err)
}

func TestInsertPaymentApplicationOutbox_MissingDestinationErrors(t *testing.T) {
	t.Parallel()

	uc := &UseCase{idGenerator: testIDGenerator{}}
	err := uc.insertPaymentApplicationOutbox(
		context.Background(),
		mock.NewOutboxTxQueryer(t),
		paymentapplication.Event{
			EventID: "evt",
			Data: paymentapplication.EventData{
				FinancialPaymentIntentID: "pi_1",
			},
		},
		time.Now().UTC(),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "destination is not configured")
}

func TestNewPaymentApplicationEventIDUsesInjectedGenerator(t *testing.T) {
	t.Parallel()

	uc := &UseCase{idGenerator: testIDGenerator{value: "generated-id-42"}}
	require.Equal(t, "generated-id-42", uc.newPaymentApplicationEventID())
}

func TestInsertPaymentApplicationOutbox_InsertFailurePropagates(t *testing.T) {
	t.Parallel()

	uc := &UseCase{
		paymentApplicationResultsDestination: testPaymentApplicationResultsDestination,
		idGenerator:                          testIDGenerator{},
	}

	outboxQ := mock.NewOutboxTxQueryer(t)
	outboxQ.EXPECT().Create(testifymock.Anything, testifymock.Anything).
		Return(model.OutboxMessage{}, errors.New("outbox insert failed")).Once()

	paidAmount := uint64(1000)
	currency := model.IRRCurrency
	paidAt := time.Now().UTC()

	err := uc.insertPaymentApplicationOutbox(
		context.Background(),
		outboxQ,
		paymentapplication.NewEvent(
			"evt",
			"cause",
			nil,
			paidAt,
			paymentapplication.EventData{
				FinancialPaymentIntentID: "pi_1",
				Result:                   paymentapplication.AppliedApplicationResult,
				PaidAmountMinor:          &paidAmount,
				Currency:                 &currency,
				FinancialPaidAt:          &paidAt,
			},
		),
		paidAt,
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "insert payment application result outbox message")
}

func TestNew_RequiredDependencies(t *testing.T) {
	t.Parallel()

	newUC := func(idGenerator identifier.Generator, destination string, auditDestination string) (*UseCase, error) {
		return New(
			mock.NewPool(t),
			&contractuc.UseCase{},
			noopCheckoutSessionPaymentHook{},
			mock.NewCheckoutPaymentClient(t),
			mock.NewInbox(t),
			mock.NewOutbox(t),
			mock.NewPaymentAttempt(t),
			mock.NewContract(t),
			mock.NewInstallment(t),
			mock.NewInstallmentCharge(t),
			mock.NewCreditAccount(t),
			idGenerator,
			testFingerprintHasher(),
			destination,
			auditDestination,
			mock.NewClock(t),
			0,
			metricsnoop.New(),
		)
	}

	t.Run("missing id generator rejected", func(t *testing.T) {
		t.Parallel()

		_, err := newUC(nil, testPaymentApplicationResultsDestination, testAuditDestination)
		require.Error(t, err)
		require.Contains(t, err.Error(), "id generator")
	})

	t.Run("missing payment application results destination rejected", func(t *testing.T) {
		t.Parallel()

		_, err := newUC(testIDGenerator{}, "", testAuditDestination)
		require.Error(t, err)
		require.Contains(t, err.Error(), "payment application results destination")
	})

	t.Run("missing audit destination rejected", func(t *testing.T) {
		t.Parallel()

		_, err := newUC(testIDGenerator{}, testPaymentApplicationResultsDestination, "")
		require.Error(t, err)
		require.Contains(t, err.Error(), "audit destination")
	})
}
