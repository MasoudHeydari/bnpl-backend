package financialcorepaymentresultuc

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/event/audit"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/trace"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/contractuc"
)

func testAuditCtx() context.Context {
	ctx := context.Background()
	ctx = trace.WithTraceID(ctx, "trace-payment-result")
	ctx = trace.WithCorrelationID(ctx, "corr-context")
	return ctx
}

func sampleProcessInput(contractID model.ContractID, installmentID model.InstallmentID) ProcessInput {
	corr := "corr-input"
	upstream := "upstream-cause"
	return ProcessInput{
		EventID:          "fc-event-1",
		EventType:        "financial_core.payment_result",
		EventVersion:     "1",
		SourceService:    "financial-core",
		CorrelationID:    &corr,
		CausationID:      &upstream,
		PaymentPurpose:   model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
		ContractID:       contractID,
		InstallmentID:    installmentID,
		InvoiceID:        "inv-1",
		PaymentIntentID:  "pi-1",
		PaymentAttemptID: "fc-attempt-1",
		PaymentSessionID: "ps-1",
		PaymentStatus:    model.SucceededFinancialPaymentResultStatus,
		PaidAmountMinor:  1500,
		Currency:         model.IRRCurrency,
		PaidAt:           time.Date(2026, 7, 14, 11, 0, 0, 0, time.UTC),
		EventOccurredAt:  time.Date(2026, 7, 14, 11, 1, 0, 0, time.UTC),
		SourceType:       "contract",
		SourceID:         contractID.String(),
		RawPayload:       json.RawMessage(`{"secret":"do-not-audit"}`),
	}
}

func TestBuildCheckoutActivationAuditEvent(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	contractID := model.ContractID(uuid.New())
	decisionID := model.DecisionID(uuid.New())
	customerID := model.CustomerID(uuid.New())
	merchantProfileID := model.MerchantProfileID(uuid.New())
	branchProfileID := model.MerchantBranchProfileID(uuid.New())
	firstID := model.InstallmentID(uuid.New())
	remainingScheduledID := model.InstallmentID(uuid.New())
	remainingDueID := model.InstallmentID(uuid.New())
	attemptID := model.PaymentAttemptID(uuid.New())
	inboxID := model.InboxID(uuid.New())
	activatedAt := now

	input := sampleProcessInput(contractID, model.InstallmentID{})
	facts := checkoutActivationAuditFacts{
		InboxID: inboxID,
		ActivateOutput: contractuc.ActivateOutput{
			Result: contractuc.ActivateResultActivated,
			Contract: model.Contract{
				ID:                            contractID,
				DecisionID:                    decisionID,
				CustomerID:                    customerID,
				MerchantProfileID:             merchantProfileID,
				MerchantBranchProfileID:       &branchProfileID,
				Status:                        model.ActiveContractStatus,
				RemainingPrincipalAmountMinor: 9000,
				ActivatedAt:                   &activatedAt,
			},
			FirstInstallment: model.Installment{
				ID:     firstID,
				Status: model.PaidInstallmentStatus,
			},
			RemainingInstallments: []model.Installment{
				{ID: remainingScheduledID, Status: model.ScheduledInstallmentStatus},
				{ID: remainingDueID, Status: model.DueInstallmentStatus},
			},
			PaidChargeCount:                2,
			AppliedChargeCount:             3,
			AddedActiveExposureAmountMinor: 10000,
		},
		Attempt:   model.PaymentAttempt{ID: attemptID, ContractID: contractID},
		AppliedAt: now,
	}

	event, err := buildCheckoutActivationAuditEvent("audit-1", "corr-input", "trace-1", input, facts, now)
	require.NoError(t, err)
	require.NoError(t, audit.ValidateEvent(event))
	require.Equal(t, auditActionContractActivateFromCheckoutPayment, event.Data.Action)
	require.Equal(t, contractResourceType, event.Data.Resource.Type)
	require.Equal(t, contractID.String(), event.Data.Resource.ID)
	require.Equal(t, audit.ServiceActorType, event.Data.Actor.Type)
	require.NotNil(t, event.Data.Actor.Service)
	require.Equal(t, "financial-core", *event.Data.Actor.Service)
	require.NotNil(t, event.CausationEventID)
	require.Equal(t, "fc-event-1", *event.CausationEventID)
	require.Equal(t, auditReasonCheckoutFirstInstallmentPaymentSucceeded, *event.Data.ReasonCode)
	require.Equal(t, []string{auditReasonCheckoutFirstInstallmentPaymentSucceeded}, event.Data.Metadata["reason_codes"])
	require.Equal(t, "upstream-cause", event.Data.Metadata["upstream_causation_id"])
	require.NotContains(t, event.Data.Metadata, "raw_payload")
	require.Nil(t, event.Data.Metadata["RawPayload"])

	require.Len(t, event.Data.Changes, 3)
	require.Equal(t, "contract.status", event.Data.Changes[0].Field)
	require.Equal(t, "first_installment.status", event.Data.Changes[1].Field)
	require.Equal(t, "remaining_installments.status", event.Data.Changes[2].Field)
	require.Equal(t, 1, event.Data.Metadata["scheduled_installment_count"])
	require.Equal(t, 1, event.Data.Metadata["due_installment_count"])
	require.Equal(t, attemptID.String(), event.Data.Metadata["payment_attempt_id"])
	require.Equal(t, "fc-attempt-1", event.Data.Metadata["financial_core_payment_attempt_id"])
}

func TestBuildInstallmentRepaymentAuditEvent_IncludesContractCompletion(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	contractID := model.ContractID(uuid.New())
	installmentID := model.InstallmentID(uuid.New())
	attemptID := model.PaymentAttemptID(uuid.New())
	input := sampleProcessInput(contractID, installmentID)
	input.PaymentPurpose = model.InstallmentRepaymentFinancialCorePaymentPurpose

	event, err := buildInstallmentRepaymentAuditEvent(
		"audit-2",
		"corr-input",
		"trace-1",
		input,
		installmentRepaymentAuditFacts{
			InboxID:                   model.InboxID(uuid.New()),
			PreviousInstallmentStatus: model.DueInstallmentStatus,
			CurrentInstallment: model.Installment{
				ID:               installmentID,
				InstallmentOrder: 3,
				Status:           model.PaidInstallmentStatus,
			},
			PreviousPaymentAttemptState: model.SubmittedPaymentAttemptState,
			CurrentPaymentAttempt: model.PaymentAttempt{
				ID:    attemptID,
				State: model.SucceededPaymentAttemptState,
			},
			PreviousContractStatus:  model.ActiveContractStatus,
			CurrentContractStatus:   model.CompletedContractStatus,
			RemainingPrincipalMinor: 0,
			ContractCompleted:       true,
			AppliedAt:               now,
			CustomerID:              model.CustomerID(uuid.New()),
			ContractID:              contractID,
		},
		now,
	)
	require.NoError(t, err)
	require.NoError(t, audit.ValidateEvent(event))
	require.Equal(t, installmentResourceType, event.Data.Resource.Type)
	require.Equal(t, installmentID.String(), event.Data.Resource.ID)
	require.Equal(t, auditActionInstallmentApplyRepayment, event.Data.Action)
	require.Len(t, event.Data.Changes, 3)
	require.Equal(t, "contract.status", event.Data.Changes[2].Field)
	require.Equal(t, true, event.Data.Metadata["contract_completed"])
}

func TestBuildPaymentAttemptFailedAndCanceledAuditEvents(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	attemptID := model.PaymentAttemptID(uuid.New())
	contractID := model.ContractID(uuid.New())
	input := sampleProcessInput(contractID, model.InstallmentID{})
	input.PaymentStatus = model.FailedFinancialPaymentResultStatus

	failed, err := buildPaymentAttemptFailedAuditEvent(
		"audit-fail",
		"corr",
		"trace",
		input,
		paymentAttemptTerminalAuditFacts{
			InboxID:       model.InboxID(uuid.New()),
			Attempt:       model.PaymentAttempt{ID: attemptID},
			PreviousState: model.SubmittedPaymentAttemptState,
			CurrentState:  model.FailedPaymentAttemptState,
			ProcessResult: ProcessResultFailureRecorded,
			ReasonCode:    "gateway_declined",
			ContractID:    contractID,
		},
		now,
	)
	require.NoError(t, err)
	require.Equal(t, audit.FailedOutcome, failed.Data.Outcome)
	require.Equal(t, "gateway_declined", *failed.Data.ReasonCode)
	require.Equal(t, []string{"gateway_declined"}, failed.Data.Metadata["reason_codes"])
	require.Equal(t, paymentAttemptResourceType, failed.Data.Resource.Type)

	canceled, err := buildPaymentAttemptCanceledAuditEvent(
		"audit-cancel",
		"corr",
		"trace",
		input,
		paymentAttemptTerminalAuditFacts{
			InboxID:       model.InboxID(uuid.New()),
			Attempt:       model.PaymentAttempt{ID: attemptID},
			PreviousState: model.SubmittedPaymentAttemptState,
			CurrentState:  model.CanceledPaymentAttemptState,
			ProcessResult: ProcessResultCancellationRecorded,
			ContractID:    contractID,
		},
		now,
	)
	require.NoError(t, err)
	// Cancellation recording succeeded; payment itself did not.
	require.Equal(t, audit.SucceededOutcome, canceled.Data.Outcome)
	require.Equal(t, auditReasonPaymentAttemptCanceled, *canceled.Data.ReasonCode)
	require.Equal(t, []string{auditReasonPaymentAttemptCanceled}, canceled.Data.Metadata["reason_codes"])
}

func TestBuildReconciliationAndUnmatchedAuditEvents(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	attemptID := model.PaymentAttemptID(uuid.New())
	contractID := model.ContractID(uuid.New())
	input := sampleProcessInput(contractID, model.InstallmentID{})
	attempt := model.PaymentAttempt{ID: attemptID}
	prev := model.SubmittedPaymentAttemptState
	curr := model.ReconciliationRequiredPaymentAttemptState

	recon, err := buildReconciliationAuditEvent(
		"audit-recon",
		"corr",
		"trace",
		input,
		reconciliationAuditFacts{
			InboxID:                     model.InboxID(uuid.New()),
			Reason:                      model.UnknownPaymentAttemptPaymentReconciliationReason,
			Attempt:                     &attempt,
			PreviousPaymentAttemptState: &prev,
			CurrentPaymentAttemptState:  &curr,
			ContractID:                  contractID,
			AttemptStateChanged:         true,
		},
		now,
	)
	require.NoError(t, err)
	require.Equal(t, auditActionPaymentResultRequireReconciliation, recon.Data.Action)
	require.Equal(t, audit.FailedOutcome, recon.Data.Outcome)
	require.Equal(t, string(model.UnknownPaymentAttemptPaymentReconciliationReason), *recon.Data.ReasonCode)
	require.Equal(t, []string{string(model.UnknownPaymentAttemptPaymentReconciliationReason)}, recon.Data.Metadata["reason_codes"])
	require.Equal(t, "payment_attempt.state", recon.Data.Changes[0].Field)

	unmatched, err := buildUnmatchedTerminalAuditEvent(
		"audit-unmatched",
		"corr",
		"trace",
		input,
		unmatchedTerminalAuditFacts{
			InboxID:     model.InboxID(uuid.New()),
			FailureCode: "unknown_payment_attempt",
			ContractID:  contractID,
		},
		now,
	)
	require.NoError(t, err)
	require.Equal(t, auditActionPaymentResultRecordUnmatchedTerminalResult, unmatched.Data.Action)
	require.Equal(t, contractResourceType, unmatched.Data.Resource.Type)
	require.Equal(t, "unknown_payment_attempt", *unmatched.Data.ReasonCode)
	require.Equal(t, []string{"unknown_payment_attempt"}, unmatched.Data.Metadata["reason_codes"])
	require.NotContains(t, unmatched.Data.Metadata, "installment_id")
}

func TestResolveAuditEnvelopeIDs_PrefersInputCorrelation(t *testing.T) {
	t.Parallel()

	corr, traceID := resolveAuditEnvelopeIDs(testAuditCtx(), sampleProcessInput(model.ContractID(uuid.New()), model.InstallmentID{}))
	require.Equal(t, "corr-input", corr)
	require.Equal(t, "trace-payment-result", traceID)

	input := sampleProcessInput(model.ContractID(uuid.New()), model.InstallmentID{})
	input.CorrelationID = nil
	corr, _ = resolveAuditEnvelopeIDs(testAuditCtx(), input)
	require.Equal(t, "corr-context", corr)
}

func TestInsertAuditOutbox_UsesResourceMessageKeyAndDestination(t *testing.T) {
	t.Parallel()

	ctx := testAuditCtx()
	now := time.Now().UTC()
	outboxQ := mock.NewOutboxTxQueryer(t)
	contractID := model.ContractID(uuid.New())
	input := sampleProcessInput(contractID, model.InstallmentID{})

	event, err := buildCheckoutActivationAuditEvent(
		"audit-outbox-1",
		"corr-input",
		"trace-payment-result",
		input,
		checkoutActivationAuditFacts{
			InboxID: model.InboxID(uuid.New()),
			ActivateOutput: contractuc.ActivateOutput{
				Contract: model.Contract{
					ID:                contractID,
					DecisionID:        model.DecisionID(uuid.New()),
					CustomerID:        model.CustomerID(uuid.New()),
					MerchantProfileID: model.MerchantProfileID(uuid.New()),
					Status:            model.ActiveContractStatus,
				},
				FirstInstallment: model.Installment{ID: model.InstallmentID(uuid.New()), Status: model.PaidInstallmentStatus},
			},
			Attempt:   model.PaymentAttempt{ID: model.PaymentAttemptID(uuid.New())},
			AppliedAt: now,
		},
		now,
	)
	require.NoError(t, err)

	outboxQ.EXPECT().Create(ctx, testifymock.MatchedBy(func(message repo.CreateOutboxMessage) bool {
		require.Equal(t, testAuditDestination, message.Destination)
		require.Equal(t, contractID.String(), message.MessageKey)
		require.Equal(t, audit.EventType, message.EventType)
		require.NotNil(t, message.AggregateType)
		require.Equal(t, contractResourceType, *message.AggregateType)

		var decoded audit.Event
		require.NoError(t, json.Unmarshal(message.Payload, &decoded))
		require.NoError(t, audit.ValidateEvent(decoded))
		require.NotContains(t, string(message.Payload), "do-not-audit")
		return true
	})).Return(model.OutboxMessage{}, nil).Once()

	uc := &UseCase{auditDestination: testAuditDestination}
	require.NoError(t, uc.insertAuditOutbox(ctx, outboxQ, event, now))
}

func TestInsertAuditOutbox_InsertFailurePropagates(t *testing.T) {
	t.Parallel()

	ctx := testAuditCtx()
	now := time.Now().UTC()
	outboxQ := mock.NewOutboxTxQueryer(t)
	outboxQ.EXPECT().Create(testifymock.Anything, testifymock.Anything).
		Return(model.OutboxMessage{}, errors.New("outbox down")).Once()

	event, err := buildPayloadConflictAuditEvent(
		"audit-x",
		"corr",
		"trace",
		sampleProcessInput(model.ContractID(uuid.New()), model.InstallmentID{}),
		model.InboxID(uuid.New()),
		now,
	)
	require.NoError(t, err)

	uc := &UseCase{auditDestination: testAuditDestination}
	err = uc.insertAuditOutbox(ctx, outboxQ, event, now)
	require.Error(t, err)
	require.Contains(t, err.Error(), "insert payment result audit outbox message")
}

func TestFinishCheckoutTerminal_AlreadyFailedCreatesNoAudit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Now().UTC()
	contractID := model.ContractID(uuid.New())
	attemptID := model.PaymentAttemptID(uuid.New())
	inboxID := model.InboxID(uuid.New())
	intentID := "intent-already-failed"

	attempt := model.PaymentAttempt{
		ID:                       attemptID,
		ContractID:               contractID,
		Purpose:                  model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
		State:                    model.FailedPaymentAttemptState,
		FinancialPaymentIntentID: &intentID,
	}

	tx := mock.NewTx(t)
	paymentAttemptRepo := mock.NewPaymentAttempt(t)
	paymentAttemptQ := mock.NewPaymentAttemptTxQueryer(t)
	contractRepo := mock.NewContract(t)
	contractQ := mock.NewContractTxQueryer(t)
	installmentRepo := mock.NewInstallment(t)
	installmentQ := mock.NewInstallmentTxQueryer(t)
	installmentChargeRepo := mock.NewInstallmentCharge(t)
	installmentChargeQ := mock.NewInstallmentChargeTxQueryer(t)
	inboxQ := mock.NewInboxTxQueryer(t)
	outboxQ := mock.NewOutboxTxQueryer(t)

	paymentAttemptRepo.EXPECT().Tx(tx).Return(paymentAttemptQ).Once()
	paymentAttemptQ.EXPECT().GetByFinancialPaymentIntentIDWithItems(ctx, intentID).Return(attempt, nil).Once()
	contractRepo.EXPECT().Tx(tx).Return(contractQ).Once()
	contractQ.EXPECT().LockByID(ctx, contractID).Return(model.Contract{
		ID:     contractID,
		Status: model.PaymentPendingContractStatus,
	}, nil).Once()
	paymentAttemptQ.EXPECT().LockByIDWithItems(ctx, attemptID).Return(attempt, nil).Once()
	contractQ.EXPECT().MarkCanceledFromPaymentPending(ctx, contractID, now, now).
		Return(model.Contract{}, nil).Once()
	installmentRepo.EXPECT().Tx(tx).Return(installmentQ).Once()
	installmentQ.EXPECT().ExpireOfferedByContractID(ctx, contractID, now).Return(int64(0), nil).Once()
	installmentChargeRepo.EXPECT().Tx(tx).Return(installmentChargeQ).Once()
	installmentChargeQ.EXPECT().CancelOfferedByContractID(ctx, contractID, now).Return(int64(0), nil).Once()
	inboxQ.EXPECT().MarkProcessed(ctx, inboxID, now, now).
		Return(model.Inbox{ID: inboxID, Status: model.ProcessedInboxStatus}, nil).Once()

	uc := &UseCase{
		paymentAttempt:    paymentAttemptRepo,
		contract:          contractRepo,
		installment:       installmentRepo,
		installmentCharge: installmentChargeRepo,
		checkoutSessionUC: noopCheckoutSessionPaymentHook{},
		idGenerator:       testIDGenerator{},
		auditDestination:  testAuditDestination,
	}
	var output ProcessOutput
	err := uc.finishCheckoutTerminalPaymentResult(
		ctx,
		tx,
		inboxQ,
		outboxQ,
		inboxID,
		ProcessInput{
			EventID:         "fc-dup",
			EventType:       "payment.result",
			EventVersion:    "1",
			SourceService:   "financial-core",
			ContractID:      contractID,
			PaymentIntentID: intentID,
			PaymentStatus:   model.FailedFinancialPaymentResultStatus,
			EventOccurredAt: now,
		},
		now,
		ProcessResultFailureRecorded,
		&output,
	)
	require.NoError(t, err)
	require.Equal(t, ProcessResultEventAlreadyProcessed, output.Result)
}

func TestAuditActor_FallsBackToFinancialCoreServiceName(t *testing.T) {
	t.Parallel()

	actor := auditActor("")
	require.Equal(t, audit.ServiceActorType, actor.Type)
	require.NotNil(t, actor.Service)
	require.Equal(t, "financial-core", *actor.Service)
}
