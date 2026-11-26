package customercredituc_test

import (
	"context"
	"encoding/json"
	"errors"
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
	extcustomer "github.com/parstis/bnpl-backend/pkg/core/external/customer"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy/initialcreditlimit"
	initialcreditcatalog "github.com/parstis/bnpl-backend/pkg/core/policy/initialcreditlimit/catalog"
	initialcreditlimitv1 "github.com/parstis/bnpl-backend/pkg/core/policy/initialcreditlimit/v1"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/trace"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/customercredituc"
)

const assignTestMobileE164 = "+989121234567"

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
	customerResolver   *mock.CustomerResolver
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
		customerResolver:   mock.NewCustomerResolver(t),
		clock:              adapterclock.NewFixed(now, time.UTC),
		registry:           registry,
		defaultPolicy:      policy,
	}
}

func (d assignInitialLimitDeps) expectResolveActive(
	ctx context.Context,
	externalCustomerID model.ExternalCustomerID,
) {
	d.t.Helper()

	d.customerResolver.EXPECT().ResolveByMobile(
		ctx,
		extcustomer.ResolveByMobileInput{MobileE164: assignTestMobileE164},
	).Return(extcustomer.ResolveByMobileOutput{
		ExternalCustomerID: externalCustomerID,
		Status:             extcustomer.ActiveStatus,
		NationalCode:       "0012345678",
	}, nil).Once()
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
		d.customerResolver,
		d.policyAssignment,
		d.registry,
		d.defaultPolicy.Ref(),
		"bnpl.audit.events.v1",
		d.idGenerator,
		collectionsv2.New(time.UTC),
		newTestObserver(d.t),
		absentTrustedMobile{},
		noExistingSharedCredit{},
		disabledCreditAllocation{},
		initialcreditcatalog.MonthlyRef(),
		mock.NewLogger(d.t),
	)
	if err != nil {
		panic(err)
	}

	return uc
}

func (d assignInitialLimitDeps) expectPolicyAssignmentDefault(ctx context.Context) {
	d.policyAssignment.EXPECT().Tx(d.tx).Return(d.policyAssignmentTx).Once()
	d.policyAssignmentTx.EXPECT().GetActiveForCustomerOrOwner(
		ctx,
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
		CreditOwnerID:      fixtureCreditOwnerID,
		ExternalCustomerID: externalCustomerID,
		Jurisdiction:       model.IRJurisdiction,
		Status:             model.ActiveCustomerStatus,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
}

func TestAssignInitialLimit_MissingProfile_CreatesPendingDurableRequest(t *testing.T) {
	t.Parallel()

	ctx := testRequestContext()
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	deps := newAssignInitialLimitDeps(t, now)
	externalCustomerID := model.ExternalCustomerID(uuid.New())
	customerID := model.CustomerID(uuid.New())
	profile := sampleActiveProfile(customerID, externalCustomerID, now)

	deps.expectConnTx(ctx)
	deps.customerProfile.EXPECT().Tx(deps.tx).Return(deps.customerProfileTx).Once()
	deps.customerProfileTx.EXPECT().EnsureExists(
		ctx,
		testifymock.Anything,
		externalCustomerID,
		model.IRJurisdiction,
		model.ActiveCustomerStatus,
		now,
		now,
		"0012345678",
	).Return(profile, true, nil).Once()
	deps.expectResolveActive(ctx, externalCustomerID)

	uc := deps.newUseCase()
	output, err := uc.AssignInitialLimit(ctx, customercredituc.AssignInitialLimitInput{
		SubjectID:  model.SubjectID(uuid.New()),
		MobileE164: assignTestMobileE164,
	})
	require.NoError(t, err)
	require.Equal(t, customercredituc.AssignInitialLimitResultPending, output.Result)
	require.True(t, output.RequestCreated)
	require.NotNil(t, output.CreditRequest)
	require.Equal(t, model.PendingCreditRequestStatus, output.CreditRequest.Status)
	require.Nil(t, output.CreditLimit)
	require.True(t, output.Decision.ID.IsZero())
}

func TestAssignInitialLimit_ExistingNonActiveProfile_IsConflictWithoutRequest(t *testing.T) {
	t.Parallel()

	ctx := testRequestContext()
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	deps := newAssignInitialLimitDeps(t, now)
	externalCustomerID := model.ExternalCustomerID(uuid.New())
	customerID := model.CustomerID(uuid.New())
	profile := sampleActiveProfile(customerID, externalCustomerID, now)
	profile.Status = model.TerminatedCustomerStatus

	deps.expectConnTx(ctx)
	deps.customerProfile.EXPECT().Tx(deps.tx).Return(deps.customerProfileTx).Once()
	deps.customerProfileTx.EXPECT().EnsureExists(
		ctx,
		testifymock.Anything,
		externalCustomerID,
		model.IRJurisdiction,
		model.ActiveCustomerStatus,
		now,
		now,
		"0012345678",
	).Return(profile, false, nil).Once()
	deps.expectResolveActive(ctx, externalCustomerID)

	uc := deps.newUseCase()
	_, err := uc.AssignInitialLimit(ctx, customercredituc.AssignInitialLimitInput{
		SubjectID:  model.SubjectID(uuid.New()),
		MobileE164: assignTestMobileE164,
	})
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeConflict))
	require.Equal(t, model.TerminatedCustomerStatus, profile.Status)
}

func TestAssignInitialLimit_EnsureExistsFailure_ReturnsError(t *testing.T) {
	t.Parallel()

	ctx := testRequestContext()
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	deps := newAssignInitialLimitDeps(t, now)
	externalCustomerID := model.ExternalCustomerID(uuid.New())
	repoErr := cerr.InternalError(errors.New("db unavailable"))

	deps.expectConnTx(ctx)
	deps.customerProfile.EXPECT().Tx(deps.tx).Return(deps.customerProfileTx).Once()
	deps.customerProfileTx.EXPECT().EnsureExists(
		ctx,
		testifymock.Anything,
		externalCustomerID,
		model.IRJurisdiction,
		model.ActiveCustomerStatus,
		now,
		now,
		"0012345678",
	).Return(model.CustomerProfile{}, false, repoErr).Once()
	deps.expectResolveActive(ctx, externalCustomerID)

	uc := deps.newUseCase()
	_, err := uc.AssignInitialLimit(ctx, customercredituc.AssignInitialLimitInput{
		SubjectID:  model.SubjectID(uuid.New()),
		MobileE164: assignTestMobileE164,
	})
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeInternalError))
}

