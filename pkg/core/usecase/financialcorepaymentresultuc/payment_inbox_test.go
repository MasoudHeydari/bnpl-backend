package financialcorepaymentresultuc

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestAcquireInboxEvent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	payload := json.RawMessage(`{"event_id":"evt-2"}`)
	subjectType := string(model.PaymentAttemptInboxSubjectType)
	subjectID := model.SubjectID(uuid.New()).String()
	extRefType := string(model.FinancialPaymentIntentInboxExternalReferenceType)
	extRef := "pi_123"

	uc := &UseCase{fingerprintHasher: testFingerprintHasher()}

	input := ProcessInput{
		EventID:       "evt-2",
		EventType:     "t",
		EventVersion:  "1",
		SourceService: "svc",
		RawPayload:    payload,
	}
	purpose := model.CheckoutFirstInstallmentFinancialCorePaymentPurpose
	expectedFingerprint, err := uc.paymentResultPayloadFingerprint(input, purpose)
	require.NoError(t, err)

	t.Run("insert success returns new", func(t *testing.T) {
		t.Parallel()

		inboxQ := mock.NewInboxTxQueryer(t)
		inserted := model.Inbox{ID: model.InboxID(uuid.New()), Status: model.ReceivedInboxStatus}

		inboxQ.EXPECT().AcquireForProcessing(
			ctx,
			testifymock.MatchedBy(func(event model.InboxEvent) bool {
				return event.EventID == "evt-2" &&
					event.PayloadFingerprint == expectedFingerprint &&
					event.ConsumerName == model.FinancialCorePaymentResultInboxConsumerName
			}),
			now,
		).Return(inserted, model.InboxAcquireNew, nil).Once()

		acquire, err := uc.acquireInboxEvent(
			ctx, inboxQ, input, purpose, &subjectType, &subjectID, &extRefType, &extRef, now,
		)
		require.NoError(t, err)
		assert.Equal(t, model.InboxAcquireNew, acquire.Result)
	})

	t.Run("processed returns already processed", func(t *testing.T) {
		t.Parallel()

		inboxQ := mock.NewInboxTxQueryer(t)
		inboxQ.EXPECT().AcquireForProcessing(ctx, testifymock.Anything, now).
			Return(model.Inbox{Status: model.ProcessedInboxStatus}, model.InboxAcquireAlreadyProcessed, nil).Once()

		acquire, err := uc.acquireInboxEvent(
			ctx, inboxQ, input, purpose, &subjectType, &subjectID, &extRefType, &extRef, now,
		)
		require.NoError(t, err)
		assert.Equal(t, model.InboxAcquireAlreadyProcessed, acquire.Result)
	})

	t.Run("acquire error propagates", func(t *testing.T) {
		t.Parallel()

		inboxQ := mock.NewInboxTxQueryer(t)
		acquireErr := errors.New("database unavailable")
		inboxQ.EXPECT().AcquireForProcessing(ctx, testifymock.Anything, now).
			Return(model.Inbox{}, model.InboxAcquireResult(""), acquireErr).Once()

		_, err := uc.acquireInboxEvent(
			ctx, inboxQ, input, purpose, &subjectType, &subjectID, &extRefType, &extRef, now,
		)
		require.ErrorIs(t, err, acquireErr)
	})
}

func TestPersistPermanentInboxFailure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	uc := &UseCase{}

	t.Run("received marks failed", func(t *testing.T) {
		t.Parallel()

		inboxQ := mock.NewInboxTxQueryer(t)
		inboxID := model.InboxID(uuid.New())

		inboxQ.EXPECT().MarkFailed(
			ctx,
			inboxID,
			"unsupported_payment_result_status",
			"unsupported payment status",
			now,
			now,
		).Return(model.Inbox{Status: model.FailedInboxStatus}, nil).Once()

		var output ProcessOutput
		err := uc.persistPermanentInboxFailure(
			ctx,
			inboxQ,
			model.Inbox{ID: inboxID, Status: model.ReceivedInboxStatus},
			"unsupported_payment_result_status",
			"unsupported payment status",
			now,
			&output,
		)
		require.NoError(t, err)
		require.Equal(t, ProcessResultPermanentlyRejected, output.Result)
	})

	t.Run("non received status is conflict", func(t *testing.T) {
		t.Parallel()

		inboxQ := mock.NewInboxTxQueryer(t)
		var output ProcessOutput
		err := uc.persistPermanentInboxFailure(
			ctx,
			inboxQ,
			model.Inbox{Status: model.ProcessedInboxStatus},
			"unsupported_payment_result_status",
			"unsupported",
			now,
			&output,
		)
		require.Error(t, err)
		assert.True(t, cerr.IsCode(err, cerr.CodeConflict))
	})
}

func TestValidateCheckoutPaymentResultStatus(t *testing.T) {
	t.Parallel()

	t.Run("empty status allowed for unknown external mapping", func(t *testing.T) {
		t.Parallel()

		require.NoError(t, validateCheckoutPaymentResultStatus(""))
	})

	t.Run("supported statuses accepted", func(t *testing.T) {
		t.Parallel()

		require.NoError(t, validateCheckoutPaymentResultStatus(model.SucceededFinancialPaymentResultStatus))
		require.NoError(t, validateCheckoutPaymentResultStatus(model.FailedFinancialPaymentResultStatus))
		require.NoError(t, validateCheckoutPaymentResultStatus(model.CanceledFinancialPaymentResultStatus))
	})

	t.Run("invalid non empty status rejected", func(t *testing.T) {
		t.Parallel()

		err := validateCheckoutPaymentResultStatus(model.FinancialPaymentResultStatus("pending"))
		require.Error(t, err)
	})
}
