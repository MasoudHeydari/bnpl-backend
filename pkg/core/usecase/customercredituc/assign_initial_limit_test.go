package customercredituc_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	collectionsv2 "github.com/parstis/bnpl-backend/pkg/core/policy/collections/v2"

	"github.com/google/uuid"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	adapterclock "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	sha256fingerprint "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/fingerprint/sha256"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/event/audit"
	"github.com/parstis/bnpl-backend/pkg/core/external/creditassessment"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy/initialcreditlimit"
	initialcreditlimitv1 "github.com/parstis/bnpl-backend/pkg/core/policy/initialcreditlimit/v1"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/trace"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/customercredituc"
)

func testRequestContext() context.Context {
	ctx := context.Background()
	ctx = trace.WithTraceID(ctx, "trace-test")
	ctx = trace.WithCorrelationID(ctx, "corr-test")
	return ctx
}

type assignInitialLimitDeps struct {
	t                  *testing.T
	pool               *mock.Pool
	conn               *mock.Conn
	tx                 *mock.Tx
	customerProfile    *mock.CustomerProfile
	customerProfileTx  *mock.CustomerProfileTxQueryer
	creditAccount      *mock.CreditAccount
	creditAccountTx    *mock.CreditAccountTxQueryer
	decision           *mock.Decision
	decisionTx         *mock.DecisionTxQueryer
	outbox             *mock.Outbox
	outboxTx           *mock.OutboxTxQueryer
	policyAssignment   *mock.PolicyAssignment
	policyAssignmentTx *mock.PolicyAssignmentTxQueryer
	idGenerator        *mock.IdentifierGenerator
	creditAssessment   *mock.CreditAssessmentClient
	clock              adapterclock.Fixed
	registry           *initialcreditlimit.Registry
	defaultPolicy      initialcreditlimit.Policy
}

func newAssignInitialLimitDeps(t *testing.T, now time.Time) assignInitialLimitDeps {
	t.Helper()

	policy := initialcreditlimitv1.New()
	registry, err := initialcreditlimit.NewRegistry(policy)
	if err != nil {
		panic(err)
	}

	return assignInitialLimitDeps{
		t:                  t,
		pool:               mock.NewPool(t),
		conn:               mock.NewConn(t),
		tx:                 mock.NewTx(t),
		customerProfile:    mock.NewCustomerProfile(t),
		customerProfileTx:  mock.NewCustomerProfileTxQueryer(t),
		creditAccount:      mock.NewCreditAccount(t),
		creditAccountTx:    mock.NewCreditAccountTxQueryer(t),
		decision:           mock.NewDecision(t),
		decisionTx:         mock.NewDecisionTxQueryer(t),
		outbox:             mock.NewOutbox(t),
		outboxTx:           mock.NewOutboxTxQueryer(t),
		policyAssignment:   mock.NewPolicyAssignment(t),
		policyAssignmentTx: mock.NewPolicyAssignmentTxQueryer(t),
		idGenerator:        mock.NewIdentifierGenerator(t),
		creditAssessment:   mock.NewCreditAssessmentClient(t),
		clock:              adapterclock.NewFixed(now, time.UTC),
		registry:           registry,
		defaultPolicy:      policy,
	}
}

func (d assignInitialLimitDeps) newUseCase() *customercredituc.UseCase {
	uc, err := customercredituc.New(
		d.pool,
		d.customerProfile,
		d.creditAccount,
		d.decision,
		mock.NewCustomerRestriction(d.t),
		mock.NewInstallment(d.t),
		mock.NewDefaultEpisode(d.t),
		d.outbox,
		d.clock,
		sha256fingerprint.New(),
		d.creditAssessment,
		d.policyAssignment,
		d.registry,
		d.defaultPolicy.Ref(),
		"bnpl.audit.events.v1",
		d.idGenerator,
		collectionsv2.New(time.UTC),
	)
	if err != nil {
		panic(err)
	}

	return uc
}

func (d assignInitialLimitDeps) expectPolicyAssignmentDefault(ctx context.Context) {
	d.policyAssignment.EXPECT().Tx(d.tx).Return(d.policyAssignmentTx).Once()
	d.policyAssignmentTx.EXPECT().GetActive(
		ctx,
		model.CustomerPolicySubjectType,
		testifymock.Anything,
		model.InitialCreditLimitPolicyFamily,
		testifymock.Anything,
	).Return(model.PolicyAssignment{}, cerr.ResourceNotFound(errors.New("missing"))).Once()
}

func (d assignInitialLimitDeps) expectFetchSignals(ctx context.Context) {
	d.creditAssessment.EXPECT().FetchSignals(ctx, testifymock.Anything).
		Return(creditassessment.FetchSignalsResult{}, nil).Once()
}

func (d assignInitialLimitDeps) expectConnTx(ctx context.Context) {
	d.pool.EXPECT().Conn(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.ConnHandler) error {
			return handler(ctx, d.conn)
		},
	).Once()
	d.conn.EXPECT().Tx(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.TxHandler) error {
			return handler(ctx, d.tx)
		},
	).Once()
}

func (d assignInitialLimitDeps) expectAuditOutbox(
	t *testing.T,
	ctx context.Context,
	eventID string,
	resourceType string,
	resourceID uuid.UUID,
	expectedOutcome audit.Outcome,
) *audit.Event {
	t.Helper()

	captured := new(audit.Event)
	d.outbox.EXPECT().Tx(d.tx).Return(d.outboxTx).Once()
	d.idGenerator.EXPECT().Generate().Return(eventID).Once()
	d.outboxTx.EXPECT().Create(
		ctx,
		testifymock.MatchedBy(func(message repo.CreateOutboxMessage) bool {
			if message.Destination != "bnpl.audit.events.v1" ||
				message.MessageKey != resourceID.String() ||
				message.EventType != audit.EventType ||
				message.AggregateType == nil ||
				*message.AggregateType != resourceType {
				return false
			}

			var event audit.Event
			if err := json.Unmarshal(message.Payload, &event); err != nil {
				return false
			}

			if event.EventID != eventID ||
				event.Data.Action != "credit_account.assign_initial_limit" ||
				event.Data.Resource.Type != resourceType ||
				event.Data.Resource.ID != resourceID.String() ||
				event.Data.Outcome != expectedOutcome ||
				event.TraceID != trace.ID(ctx) ||
				event.CorrelationID != trace.CorrelationID(ctx) {
				return false
			}

			if err := audit.ValidateEvent(event); err != nil {
				return false
			}

			*captured = event
			return true
		}),
	).Return(model.OutboxMessage{}, nil).Once()

	return captured
}

func sampleActiveProfile(profileID model.CustomerID, externalCustomerID model.ExternalCustomerID, now time.Time) model.CustomerProfile {
	return model.CustomerProfile{
		ID:                 profileID,
		ExternalCustomerID: externalCustomerID,
		Jurisdiction:       model.IRJurisdiction,
		Status:             model.ActiveCustomerStatus,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
}

func TestAssignInitialLimit_MissingProfile_CreatesActiveProfileAndAssigns(t *testing.T) {
	t.Parallel()

	ctx := testRequestContext()
	now := time.Date(
		2026,
		7,
		22,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	deps := newAssignInitialLimitDeps(t, now)
	externalCustomerID := model.ExternalCustomerID(uuid.New())
	customerID := model.CustomerID(uuid.New())
	require.NotEqual(t, externalCustomerID.UUID(), customerID.UUID())

	profile := sampleActiveProfile(customerID, externalCustomerID, now)
	policy := initialcreditlimitv1.New()
	terms := policy.Terms()
	currency := terms.Currency
	decisionID := model.DecisionID(uuid.New())
	accountID := model.CreditAccountID(uuid.New())
	expiresAt := now.Add(terms.ValidFor)

	// prepare
	deps.expectConnTx(ctx)
	deps.expectPolicyAssignmentDefault(ctx)
	deps.customerProfile.EXPECT().Tx(deps.tx).Return(deps.customerProfileTx).Once()
	deps.creditAccount.EXPECT().Tx(deps.tx).Return(deps.creditAccountTx).Once()
	deps.customerProfileTx.EXPECT().EnsureExists(
		ctx,
		externalCustomerID,
		model.IRJurisdiction,
		model.ActiveCustomerStatus,
		now,
		now,
	).Return(profile, true, nil).Once()
	deps.creditAccountTx.EXPECT().ExpireDueByCustomerIDAndCurrency(
		ctx, customerID, currency, now,
	).Return(nil).Once()
	deps.creditAccountTx.EXPECT().LockByCustomerIDAndCurrency(
		ctx, customerID, currency,
	).Return(model.CreditAccount{}, cerr.ResourceNotFound(errors.New("missing"))).Once()

	// tryResolveIdempotentAssignment
	deps.expectConnTx(ctx)
	deps.decision.EXPECT().Tx(deps.tx).Return(deps.decisionTx).Once()
	deps.creditAccount.EXPECT().Tx(deps.tx).Return(deps.creditAccountTx).Once()
	deps.creditAccountTx.EXPECT().CountLimitSetTransactions(
		ctx, customerID, currency,
	).Return(int64(0), nil).Once()
	deps.decisionTx.EXPECT().GetByIdempotencyKey(ctx, testifymock.Anything).
		Return(model.Decision{}, cerr.ResourceNotFound(errors.New("missing"))).Once()

	// persist
	deps.expectConnTx(ctx)
	deps.customerProfile.EXPECT().Tx(deps.tx).Return(deps.customerProfileTx).Once()
	deps.creditAccount.EXPECT().Tx(deps.tx).Return(deps.creditAccountTx).Once()
	deps.decision.EXPECT().Tx(deps.tx).Return(deps.decisionTx).Once()
	deps.customerProfileTx.EXPECT().LockByID(ctx, customerID).Return(profile, nil).Once()
	deps.creditAccountTx.EXPECT().ExpireDueByCustomerIDAndCurrency(
		ctx, customerID, currency, now,
	).Return(nil).Once()
	deps.creditAccountTx.EXPECT().LockByCustomerIDAndCurrency(
		ctx, customerID, currency,
	).Return(model.CreditAccount{}, cerr.ResourceNotFound(errors.New("missing"))).Once()
	deps.creditAccountTx.EXPECT().CountLimitSetTransactions(
		ctx, customerID, currency,
	).Return(int64(0), nil).Once()
	deps.decisionTx.EXPECT().GetByIdempotencyKey(ctx, testifymock.Anything).
		Return(model.Decision{}, cerr.ResourceNotFound(errors.New("missing"))).Once()
	deps.decisionTx.EXPECT().Create(
		ctx,
		model.CreditLimitDecisionType,
		model.CustomerProfileDecisionTargetType,
		customerID.UUID(),
		model.LimitSetDecisionOutcome,
		testifymock.Anything,
		policy.Ref().ID,
		policy.Ref().Version,
		model.IRJurisdiction,
		model.SystemDecisionActorType,
		(*uuid.UUID)(nil),
		(*model.DecisionID)(nil),
		(*time.Time)(nil),
		(*time.Time)(nil),
		now,
		now,
		testifymock.AnythingOfType("string"),
		testifymock.AnythingOfType("string"),
	).Return(model.Decision{
		ID:            decisionID,
		Outcome:       model.LimitSetDecisionOutcome,
		ReasonCodes:   []string{terms.ApprovalReasonCode},
		PolicyID:      policy.Ref().ID,
		PolicyVersion: policy.Ref().Version,
	}, nil).Once()
	deps.creditAccountTx.EXPECT().CreateWithLimit(
		ctx,
		customerID,
		currency,
		terms.DefaultAmountMinor,
		decisionID,
		now,
		&expiresAt,
		now,
		now,
	).Return(model.CreditAccount{
		ID:               accountID,
		CustomerID:       customerID,
		LimitAmountMinor: terms.DefaultAmountMinor,
		Currency:         currency,
		Status:           model.ActiveCreditAccountStatus,
		EffectiveFrom:    now,
		ExpiresAt:        &expiresAt,
	}, nil).Once()
	deps.creditAccountTx.EXPECT().AppendTransaction(
		ctx,
		accountID,
		model.LimitSetCreditTransactionType,
		terms.DefaultAmountMinor,
		currency,
		model.CreditAssessmentCreditTransactionReason,
		model.DecisionCreditTransactionReferenceType,
		decisionID.UUID(),
		now,
		now,
	).Return(model.CreditTransaction{}, nil).Once()
	auditEvent := deps.expectAuditOutbox(
		t,
		ctx,
		"audit-event-created",
		"credit_account",
		accountID.UUID(),
		audit.SucceededOutcome,
	)

	deps.expectFetchSignals(ctx)
	uc := deps.newUseCase()
	output, err := uc.AssignInitialLimit(ctx, customercredituc.AssignInitialLimitInput{ExternalCustomerID: externalCustomerID})
	require.NoError(t, err)
	require.Equal(t, customercredituc.AssignInitialLimitResultCreated, output.Result)
	require.NotNil(t, output.CreditLimit)
	require.Equal(t, accountID, output.CreditLimit.ID)
	require.Equal(t, customerID, output.CreditLimit.CustomerID)
	require.NotEqual(t, externalCustomerID.UUID(), output.CreditLimit.CustomerID.UUID())
	require.Equal(t, model.IRJurisdiction, profile.Jurisdiction)
	require.Equal(t, model.ActiveCustomerStatus, profile.Status)

	require.NotNil(t, auditEvent.Data.ReasonCode)
	require.Equal(t, terms.ApprovalReasonCode, *auditEvent.Data.ReasonCode)
	require.Equal(t, []any{terms.ApprovalReasonCode}, auditEvent.Data.Metadata["reason_codes"])
	require.Equal(t, customerID.String(), auditEvent.Data.Metadata["customer_id"])
	require.Equal(t, externalCustomerID.String(), auditEvent.Data.Metadata["identity_subject_id"])
	require.Equal(t, decisionID.String(), auditEvent.Data.Metadata["decision_id"])
	require.Equal(t, accountID.String(), auditEvent.Data.Metadata["credit_account_id"])
	require.Equal(t, policy.Ref().ID, auditEvent.Data.Metadata["policy_id"])
	require.Equal(t, policy.Ref().Version, auditEvent.Data.Metadata["policy_version"])
	require.Equal(t, string(model.LimitSetDecisionOutcome), fmt.Sprint(auditEvent.Data.Metadata["decision_outcome"]))
	require.EqualValues(t, terms.DefaultAmountMinor, auditEvent.Data.Metadata["limit_amount_minor"])
	require.Equal(t, string(currency), fmt.Sprint(auditEvent.Data.Metadata["currency"]))
	require.Equal(t, string(model.ActiveCreditAccountStatus), fmt.Sprint(auditEvent.Data.Metadata["status"]))
	_, hasVerification := auditEvent.Data.Metadata["identity_verification_id"]
	require.False(t, hasVerification)
	_, hasAssessmentSource := auditEvent.Data.Metadata["assessment_source"]
	require.False(t, hasAssessmentSource)
	require.Len(t, auditEvent.Data.Changes, 3)
	require.Equal(t, "decision.outcome", auditEvent.Data.Changes[0].Field)
	require.Equal(t, string(model.LimitSetDecisionOutcome), fmt.Sprint(auditEvent.Data.Changes[0].After))
	require.Equal(t, "credit_account.status", auditEvent.Data.Changes[1].Field)
	require.Nil(t, auditEvent.Data.Changes[1].Before)
	require.Equal(t, string(model.ActiveCreditAccountStatus), fmt.Sprint(auditEvent.Data.Changes[1].After))
	require.Equal(t, "credit_account.limit_amount_minor", auditEvent.Data.Changes[2].Field)
	require.EqualValues(t, terms.DefaultAmountMinor, auditEvent.Data.Changes[2].After)
}

func TestAssignInitialLimit_ExistingNonActiveProfile_IsDeniedWithoutReactivation(t *testing.T) {
	t.Parallel()

	ctx := testRequestContext()
	now := time.Date(
		2026,
		7,
		22,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	deps := newAssignInitialLimitDeps(t, now)
	externalCustomerID := model.ExternalCustomerID(uuid.New())
	customerID := model.CustomerID(uuid.New())
	require.NotEqual(t, externalCustomerID.UUID(), customerID.UUID())

	profile := sampleActiveProfile(customerID, externalCustomerID, now)
	profile.Status = model.TerminatedCustomerStatus
	decisionID := model.DecisionID(uuid.New())

	deps.expectConnTx(ctx)
	deps.expectPolicyAssignmentDefault(ctx)
	deps.customerProfile.EXPECT().Tx(deps.tx).Return(deps.customerProfileTx).Once()
	deps.creditAccount.EXPECT().Tx(deps.tx).Return(deps.creditAccountTx).Once()
	deps.customerProfileTx.EXPECT().EnsureExists(
		ctx,
		externalCustomerID,
		model.IRJurisdiction,
		model.ActiveCustomerStatus,
		now,
		now,
	).Return(profile, false, nil).Once()

	deps.expectConnTx(ctx)
	deps.decision.EXPECT().Tx(deps.tx).Return(deps.decisionTx).Once()
	deps.creditAccount.EXPECT().Tx(deps.tx).Return(deps.creditAccountTx).Once()
	deps.decisionTx.EXPECT().GetByIdempotencyKey(ctx, testifymock.Anything).
		Return(model.Decision{}, cerr.ResourceNotFound(errors.New("missing"))).Once()

	deps.expectConnTx(ctx)
	deps.customerProfile.EXPECT().Tx(deps.tx).Return(deps.customerProfileTx).Once()
	deps.creditAccount.EXPECT().Tx(deps.tx).Return(deps.creditAccountTx).Once()
	deps.decision.EXPECT().Tx(deps.tx).Return(deps.decisionTx).Once()
	deps.customerProfileTx.EXPECT().LockByID(ctx, customerID).Return(profile, nil).Once()
	deps.decisionTx.EXPECT().Create(
		ctx,
		model.CreditLimitDecisionType,
		model.CustomerProfileDecisionTargetType,
		customerID.UUID(),
		model.DeniedDecisionOutcome,
		[]string{model.CustomerProfileNotActiveDecisionReason},
		testifymock.AnythingOfType("string"),
		testifymock.AnythingOfType("string"),
		model.IRJurisdiction,
		model.SystemDecisionActorType,
		(*uuid.UUID)(nil),
		(*model.DecisionID)(nil),
		(*time.Time)(nil),
		(*time.Time)(nil),
		now,
		now,
		testifymock.AnythingOfType("string"),
		testifymock.AnythingOfType("string"),
	).Return(model.Decision{
		ID:          decisionID,
		Outcome:     model.DeniedDecisionOutcome,
		ReasonCodes: []string{model.CustomerProfileNotActiveDecisionReason},
	}, nil).Once()
	auditEvent := deps.expectAuditOutbox(
		t,
		ctx,
		"audit-event-denied",
		"customer_profile",
		customerID.UUID(),
		audit.DeniedOutcome,
	)

	uc := deps.newUseCase()
	output, err := uc.AssignInitialLimit(ctx, customercredituc.AssignInitialLimitInput{ExternalCustomerID: externalCustomerID})
	require.NoError(t, err)
	require.Equal(t, customercredituc.AssignInitialLimitResultDenied, output.Result)
	require.Nil(t, output.CreditLimit)
	require.Equal(t, model.TerminatedCustomerStatus, profile.Status)

	require.NotNil(t, auditEvent.Data.ReasonCode)
	require.Equal(t, model.CustomerProfileNotActiveDecisionReason, *auditEvent.Data.ReasonCode)
	require.Equal(t, []any{model.CustomerProfileNotActiveDecisionReason}, auditEvent.Data.Metadata["reason_codes"])
	require.Equal(t, customerID.String(), auditEvent.Data.Metadata["customer_id"])
	require.Equal(t, externalCustomerID.String(), auditEvent.Data.Metadata["identity_subject_id"])
	require.Equal(t, decisionID.String(), auditEvent.Data.Metadata["decision_id"])
	_, hasAccountID := auditEvent.Data.Metadata["credit_account_id"]
	require.False(t, hasAccountID)
	_, hasAmount := auditEvent.Data.Metadata["limit_amount_minor"]
	require.False(t, hasAmount)
	_, hasStatus := auditEvent.Data.Metadata["status"]
	require.False(t, hasStatus)
	_, hasExpires := auditEvent.Data.Metadata["expires_at"]
	require.False(t, hasExpires)
}

func TestAssignInitialLimit_EnsureExistsFailure_ReturnsError(t *testing.T) {
	t.Parallel()

	ctx := testRequestContext()
	now := time.Date(
		2026,
		7,
		22,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	deps := newAssignInitialLimitDeps(t, now)
	externalCustomerID := model.ExternalCustomerID(uuid.New())
	repoErr := cerr.InternalError(errors.New("db unavailable"))

	deps.expectConnTx(ctx)
	deps.customerProfile.EXPECT().Tx(deps.tx).Return(deps.customerProfileTx).Once()
	deps.creditAccount.EXPECT().Tx(deps.tx).Return(deps.creditAccountTx).Once()
	deps.customerProfileTx.EXPECT().EnsureExists(
		ctx,
		externalCustomerID,
		model.IRJurisdiction,
		model.ActiveCustomerStatus,
		now,
		now,
	).Return(model.CustomerProfile{}, false, repoErr).Once()

	uc := deps.newUseCase()
	_, err := uc.AssignInitialLimit(ctx, customercredituc.AssignInitialLimitInput{ExternalCustomerID: externalCustomerID})
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeInternalError))
}

// TestAssignInitialLimit_DenialPathStampsPolicyRef documents that the
// denial-path Decision is stamped with the governing initial-limit policy Ref.
func TestAssignInitialLimit_DenialPathStampsPolicyRef(t *testing.T) {
	t.Parallel()

	ctx := testRequestContext()
	now := time.Date(
		2026,
		7,
		22,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	deps := newAssignInitialLimitDeps(t, now)
	externalCustomerID := model.ExternalCustomerID(uuid.New())
	customerID := model.CustomerID(uuid.New())
	require.NotEqual(t, externalCustomerID.UUID(), customerID.UUID())

	profile := sampleActiveProfile(customerID, externalCustomerID, now)
	profile.Status = model.TerminatedCustomerStatus
	decisionID := model.DecisionID(uuid.New())
	v1Ref := initialcreditlimitv1.New().Ref()

	deps.expectConnTx(ctx)
	deps.expectPolicyAssignmentDefault(ctx)
	deps.customerProfile.EXPECT().Tx(deps.tx).Return(deps.customerProfileTx).Once()
	deps.creditAccount.EXPECT().Tx(deps.tx).Return(deps.creditAccountTx).Once()
	deps.customerProfileTx.EXPECT().EnsureExists(
		ctx,
		externalCustomerID,
		model.IRJurisdiction,
		model.ActiveCustomerStatus,
		now,
		now,
	).Return(profile, false, nil).Once()

	deps.expectConnTx(ctx)
	deps.decision.EXPECT().Tx(deps.tx).Return(deps.decisionTx).Once()
	deps.creditAccount.EXPECT().Tx(deps.tx).Return(deps.creditAccountTx).Once()
	deps.decisionTx.EXPECT().GetByIdempotencyKey(ctx, testifymock.Anything).
		Return(model.Decision{}, cerr.ResourceNotFound(errors.New("missing"))).Once()

	deps.expectConnTx(ctx)
	deps.customerProfile.EXPECT().Tx(deps.tx).Return(deps.customerProfileTx).Once()
	deps.creditAccount.EXPECT().Tx(deps.tx).Return(deps.creditAccountTx).Once()
	deps.decision.EXPECT().Tx(deps.tx).Return(deps.decisionTx).Once()
	deps.customerProfileTx.EXPECT().LockByID(ctx, customerID).Return(profile, nil).Once()
	deps.decisionTx.EXPECT().Create(
		ctx,
		model.CreditLimitDecisionType,
		model.CustomerProfileDecisionTargetType,
		customerID.UUID(),
		model.DeniedDecisionOutcome,
		[]string{model.CustomerProfileNotActiveDecisionReason},
		v1Ref.ID,
		v1Ref.Version,
		model.IRJurisdiction,
		model.SystemDecisionActorType,
		(*uuid.UUID)(nil),
		(*model.DecisionID)(nil),
		(*time.Time)(nil),
		(*time.Time)(nil),
		now,
		now,
		testifymock.AnythingOfType("string"),
		testifymock.AnythingOfType("string"),
	).Return(model.Decision{
		ID:            decisionID,
		Outcome:       model.DeniedDecisionOutcome,
		ReasonCodes:   []string{model.CustomerProfileNotActiveDecisionReason},
		PolicyID:      v1Ref.ID,
		PolicyVersion: v1Ref.Version,
	}, nil).Once()
	deps.expectAuditOutbox(
		t,
		ctx,
		"audit-event-policy-denied",
		"customer_profile",
		customerID.UUID(),
		audit.DeniedOutcome,
	)

	uc := deps.newUseCase()
	output, err := uc.AssignInitialLimit(ctx, customercredituc.AssignInitialLimitInput{ExternalCustomerID: externalCustomerID})
	require.NoError(t, err)
	require.Equal(t, customercredituc.AssignInitialLimitResultDenied, output.Result)
}
