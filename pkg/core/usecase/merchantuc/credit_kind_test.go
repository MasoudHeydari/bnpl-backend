package merchantuc_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	adapterclock "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/event/audit"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/trace"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/merchantuc"
)

const creditKindAuditDestination = "bnpl.audit.events.v1"

type creditKindHarness struct {
	pool            *mock.Pool
	conn            *mock.Conn
	tx              *mock.Tx
	merchantProfile *mock.MerchantProfile
	merchantTx      *mock.MerchantProfileTxQueryer
	merchantConn    *mock.MerchantProfileConnQueryer
	outbox          *mock.Outbox
	outboxTx        *mock.OutboxTxQueryer
	idGenerator     *mock.IdentifierGenerator
	now             time.Time
	uc              *merchantuc.CreditKindUseCase
}

func newCreditKindHarness(t *testing.T) *creditKindHarness {
	t.Helper()

	h := &creditKindHarness{
		pool:            mock.NewPool(t),
		conn:            mock.NewConn(t),
		tx:              mock.NewTx(t),
		merchantProfile: mock.NewMerchantProfile(t),
		merchantTx:      mock.NewMerchantProfileTxQueryer(t),
		merchantConn:    mock.NewMerchantProfileConnQueryer(t),
		outbox:          mock.NewOutbox(t),
		outboxTx:        mock.NewOutboxTxQueryer(t),
		idGenerator:     mock.NewIdentifierGenerator(t),
		now:             time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC),
	}

	uc, err := merchantuc.NewCreditKind(
		h.pool,
		h.merchantProfile,
		h.outbox,
		adapterclock.NewFixed(h.now, time.UTC),
		h.idGenerator,
		creditKindAuditDestination,
	)
	require.NoError(t, err)

	h.uc = uc

	return h
}

func (h *creditKindHarness) expectConnTx() {
	h.pool.EXPECT().
		Conn(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(ctx context.Context, fn repo.ConnHandler) error {
			return fn(ctx, h.conn)
		}).
		Once()
	h.conn.EXPECT().
		Tx(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(ctx context.Context, fn repo.TxHandler) error {
			return fn(ctx, h.tx)
		}).
		Once()
	h.merchantProfile.EXPECT().Tx(h.tx).Return(h.merchantTx).Once()
	h.outbox.EXPECT().Tx(h.tx).Return(h.outboxTx).Maybe()
}

func creditKindContext() context.Context {
	ctx := trace.WithTraceID(context.Background(), "trace-test")

	return trace.WithCorrelationID(ctx, "corr-test")
}

func activeMerchant() model.MerchantProfile {
	return model.MerchantProfile{
		ID:                 model.MerchantProfileID(uuid.New()),
		ExternalMerchantID: model.ExternalMerchantID(uuid.New()),
		Status:             model.ActiveMerchantStatus,
	}
}

func TestNewCreditKind_RequiresDependencies(t *testing.T) {
	t.Parallel()

	clk := adapterclock.NewFixed(time.Now(), time.UTC)
	_, err := merchantuc.NewCreditKind(nil, nil, nil, clk, nil, "")
	require.Error(t, err)
	require.ErrorContains(t, err, "merchantuc.NewCreditKind")

	_, err = merchantuc.NewCreditKind(
		mock.NewPool(t),
		mock.NewMerchantProfile(t),
		mock.NewOutbox(t),
		clk,
		mock.NewIdentifierGenerator(t),
		"",
	)
	require.ErrorContains(t, err, "audit destination is required")
}

func TestSetCreditKindEnabled_EnablesMonthlyAndWritesAudit(t *testing.T) {
	t.Parallel()

	h := newCreditKindHarness(t)
	merchant := activeMerchant()
	actor := model.SubjectID(uuid.New())
	ctx := creditKindContext()

	h.expectConnTx()
	h.merchantTx.EXPECT().LockByID(testifymock.Anything, merchant.ID).Return(merchant, nil).Once()
	h.merchantTx.EXPECT().
		IsCreditKindEnabled(testifymock.Anything, merchant.ID, model.PayNextMonthCreditKind).
		Return(false, nil).
		Once()
	h.merchantTx.EXPECT().
		SetCreditKindEnabled(testifymock.Anything, merchant.ID, model.PayNextMonthCreditKind, true, h.now).
		Return(nil).
		Once()
	h.idGenerator.EXPECT().Generate().Return("event-1").Once()

	var captured repo.CreateOutboxMessage
	h.outboxTx.EXPECT().
		Create(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(_ context.Context, message repo.CreateOutboxMessage) (model.OutboxMessage, error) {
			captured = message

			return model.OutboxMessage{}, nil
		}).
		Once()

	output, err := h.uc.SetCreditKindEnabled(ctx, merchantuc.SetCreditKindEnabledInput{
		ActorSubjectID:    actor,
		MerchantProfileID: merchant.ID,
		CreditKind:        model.PayNextMonthCreditKind,
		Enabled:           true,
	})
	require.NoError(t, err)
	require.True(t, output.Changed)
	require.True(t, output.Enabled)
	require.Equal(t, model.PayNextMonthCreditKind, output.CreditKind)
	require.Equal(t, merchant.ID, output.MerchantProfileID)

	require.Equal(t, creditKindAuditDestination, captured.Destination)
	require.Equal(t, merchant.ID.String(), captured.MessageKey)
	require.Equal(t, audit.EventType, captured.EventType)

	var event audit.Event
	require.NoError(t, json.Unmarshal(captured.Payload, &event))
	require.NoError(t, audit.ValidateEvent(event))
	require.Equal(t, "merchant_credit_kind.set_enabled", event.Data.Action)
	require.Equal(t, audit.OperatorActorType, event.Data.Actor.Type)
	require.Equal(t, actor.String(), *event.Data.Actor.ID)
	require.Equal(t, "merchant_profile", event.Data.Resource.Type)
	require.Equal(t, merchant.ID.String(), event.Data.Resource.ID)
	require.Equal(t, audit.SucceededOutcome, event.Data.Outcome)
	require.Equal(t, "corr-test", event.CorrelationID)
	require.Equal(t, "trace-test", event.TraceID)
	require.Len(t, event.Data.Changes, 1)
	require.Equal(t, "credit_kind.pay_next_month.enabled", event.Data.Changes[0].Field)
	require.Equal(t, false, event.Data.Changes[0].Before)
	require.Equal(t, true, event.Data.Changes[0].After)
}

func TestSetCreditKindEnabled_DisablesPayInInstallments(t *testing.T) {
	t.Parallel()

	h := newCreditKindHarness(t)
	merchant := activeMerchant()

	h.expectConnTx()
	h.merchantTx.EXPECT().LockByID(testifymock.Anything, merchant.ID).Return(merchant, nil).Once()
	h.merchantTx.EXPECT().
		IsCreditKindEnabled(testifymock.Anything, merchant.ID, model.PayInInstallmentsCreditKind).
		Return(true, nil).
		Once()
	h.merchantTx.EXPECT().
		SetCreditKindEnabled(testifymock.Anything, merchant.ID, model.PayInInstallmentsCreditKind, false, h.now).
		Return(nil).
		Once()
	h.idGenerator.EXPECT().Generate().Return("event-2").Once()
	h.outboxTx.EXPECT().Create(testifymock.Anything, testifymock.Anything).Return(model.OutboxMessage{}, nil).Once()

	output, err := h.uc.SetCreditKindEnabled(creditKindContext(), merchantuc.SetCreditKindEnabledInput{
		ActorSubjectID:    model.SubjectID(uuid.New()),
		MerchantProfileID: merchant.ID,
		CreditKind:        model.PayInInstallmentsCreditKind,
		Enabled:           false,
	})
	require.NoError(t, err)
	require.True(t, output.Changed)
	require.False(t, output.Enabled)
}

func TestSetCreditKindEnabled_IdempotentWhenStateUnchanged(t *testing.T) {
	t.Parallel()

	h := newCreditKindHarness(t)
	merchant := activeMerchant()

	// Legacy pay_in_installments is enabled by default, so enabling it again is a no-op:
	// no SetCreditKindEnabled call and no audit outbox message.
	h.expectConnTx()
	h.merchantTx.EXPECT().LockByID(testifymock.Anything, merchant.ID).Return(merchant, nil).Once()
	h.merchantTx.EXPECT().
		IsCreditKindEnabled(testifymock.Anything, merchant.ID, model.PayInInstallmentsCreditKind).
		Return(true, nil).
		Once()

	output, err := h.uc.SetCreditKindEnabled(creditKindContext(), merchantuc.SetCreditKindEnabledInput{
		ActorSubjectID:    model.SubjectID(uuid.New()),
		MerchantProfileID: merchant.ID,
		CreditKind:        model.PayInInstallmentsCreditKind,
		Enabled:           true,
	})
	require.NoError(t, err)
	require.False(t, output.Changed)
	require.True(t, output.Enabled)
}

func TestSetCreditKindEnabled_RejectsNonActiveMerchant(t *testing.T) {
	t.Parallel()

	for _, status := range []model.MerchantStatus{
		model.PendingReviewMerchantStatus,
		model.DeactivatedMerchantStatus,
		model.SuspendedMerchantStatus,
		model.TerminatedMerchantStatus,
	} {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()

			h := newCreditKindHarness(t)
			merchant := activeMerchant()
			merchant.Status = status

			h.expectConnTx()
			h.merchantTx.EXPECT().LockByID(testifymock.Anything, merchant.ID).Return(merchant, nil).Once()

			_, err := h.uc.SetCreditKindEnabled(creditKindContext(), merchantuc.SetCreditKindEnabledInput{
				ActorSubjectID:    model.SubjectID(uuid.New()),
				MerchantProfileID: merchant.ID,
				CreditKind:        model.PayNextMonthCreditKind,
				Enabled:           true,
			})
			require.Error(t, err)
			require.True(t, cerr.IsCode(err, cerr.CodeConflict))
		})
	}
}

func TestSetCreditKindEnabled_MerchantNotFound(t *testing.T) {
	t.Parallel()

	h := newCreditKindHarness(t)
	merchantID := model.MerchantProfileID(uuid.New())

	h.expectConnTx()
	h.merchantTx.EXPECT().
		LockByID(testifymock.Anything, merchantID).
		Return(model.MerchantProfile{}, cerr.ResourceNotFound(errors.New("missing"))).
		Once()

	_, err := h.uc.SetCreditKindEnabled(creditKindContext(), merchantuc.SetCreditKindEnabledInput{
		ActorSubjectID:    model.SubjectID(uuid.New()),
		MerchantProfileID: merchantID,
		CreditKind:        model.PayNextMonthCreditKind,
		Enabled:           true,
	})
	require.True(t, cerr.IsCode(err, cerr.CodeResourceNotFound))
}

func TestSetCreditKindEnabled_ValidatesInput(t *testing.T) {
	t.Parallel()

	h := newCreditKindHarness(t)
	valid := merchantuc.SetCreditKindEnabledInput{
		ActorSubjectID:    model.SubjectID(uuid.New()),
		MerchantProfileID: model.MerchantProfileID(uuid.New()),
		CreditKind:        model.PayNextMonthCreditKind,
		Enabled:           true,
	}

	missingActor := valid
	missingActor.ActorSubjectID = model.SubjectID{}

	missingMerchant := valid
	missingMerchant.MerchantProfileID = model.MerchantProfileID{}

	badKind := valid
	badKind.CreditKind = model.CreditKind("bnpl_plus")

	emptyKind := valid
	emptyKind.CreditKind = ""

	for name, input := range map[string]merchantuc.SetCreditKindEnabledInput{
		"missing actor":    missingActor,
		"missing merchant": missingMerchant,
		"unknown kind":     badKind,
		"empty kind":       emptyKind,
	} {
		_, err := h.uc.SetCreditKindEnabled(context.Background(), input)
		require.Truef(t, cerr.IsCode(err, cerr.CodeInvalidArgument), "%s must be invalid argument", name)
	}
}

func TestListCreditKinds_DefaultsBothKindsDisabled(t *testing.T) {
	t.Parallel()

	h := newCreditKindHarness(t)
	merchant := activeMerchant()

	h.pool.EXPECT().
		Conn(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(ctx context.Context, fn repo.ConnHandler) error {
			return fn(ctx, h.conn)
		}).
		Once()
	h.merchantProfile.EXPECT().Conn(h.conn).Return(h.merchantConn).Once()
	h.merchantConn.EXPECT().GetByID(testifymock.Anything, merchant.ID).Return(merchant, nil).Once()
	h.merchantConn.EXPECT().
		IsCreditKindEnabled(testifymock.Anything, merchant.ID, model.PayInInstallmentsCreditKind).
		Return(false, nil).
		Once()
	h.merchantConn.EXPECT().
		IsCreditKindEnabled(testifymock.Anything, merchant.ID, model.PayNextMonthCreditKind).
		Return(false, nil).
		Once()

	output, err := h.uc.ListCreditKinds(context.Background(), merchantuc.ListCreditKindsInput{
		MerchantProfileID: merchant.ID,
	})
	require.NoError(t, err)
	require.Equal(t, []merchantuc.CreditKindEligibility{
		{CreditKind: model.PayInInstallmentsCreditKind, Enabled: false},
		{CreditKind: model.PayNextMonthCreditKind, Enabled: false},
	}, output.CreditKinds)
}
