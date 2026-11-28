package customercredituc_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	adapterclock "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	sha256fingerprint "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/fingerprint/sha256"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	extcustomer "github.com/parstis/bnpl-backend/pkg/core/external/customer"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	collectionsv2 "github.com/parstis/bnpl-backend/pkg/core/policy/collections/v2"
	"github.com/parstis/bnpl-backend/pkg/core/policy/initialcreditlimit"
	initialcreditcatalog "github.com/parstis/bnpl-backend/pkg/core/policy/initialcreditlimit/catalog"
	initialcreditlimitv1 "github.com/parstis/bnpl-backend/pkg/core/policy/initialcreditlimit/v1"
	initialcreditlimitv3 "github.com/parstis/bnpl-backend/pkg/core/policy/initialcreditlimit/v3"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/customercredituc"
)

// grantingCreditAllocation drives one fixed-limit individual rule through
// submit → claim → finalize so the grant path is covered without Postgres.
type grantingCreditAllocation struct {
	mu        sync.Mutex
	entryID   uuid.UUID
	ruleID    model.CreditAllocationRuleID
	requestID model.CreditRequestID
	amount    uint64
	request   *model.CreditRequest
}

func (g *grantingCreditAllocation) Tx(repo.Tx) repo.CreditAllocationTxQueryer {
	return g
}

func (g *grantingCreditAllocation) LockConfiguration(context.Context, bool) error {
	return nil
}

func (g *grantingCreditAllocation) GetGroup(context.Context, string) (model.CreditGroup, error) {
	return model.CreditGroup{}, cerr.ResourceNotFound(errors.New("missing group"))
}

func (g *grantingCreditAllocation) ListGroups(context.Context) ([]model.CreditGroup, error) {
	return nil, nil
}

func (g *grantingCreditAllocation) PutGroup(context.Context, model.CreditGroup) (model.CreditGroup, error) {
	return model.CreditGroup{}, cerr.InternalError(errors.New("unused"))
}

func (g *grantingCreditAllocation) eligibility() model.CreditEligibilityEntry {
	return model.CreditEligibilityEntry{
		ID:           g.entryID,
		NationalCode: "0012345678",
		CreatedBy:    "test",
	}
}

func (g *grantingCreditAllocation) GetEligibilityByNationalCode(context.Context, string) (model.CreditEligibilityEntry, error) {
	return g.eligibility(), nil
}

func (g *grantingCreditAllocation) GetEligibilityByMobile(context.Context, string) (model.CreditEligibilityEntry, error) {
	return g.eligibility(), nil
}

func (g *grantingCreditAllocation) GetEligibility(context.Context, uuid.UUID) (model.CreditEligibilityEntry, error) {
	return g.eligibility(), nil
}

func (g *grantingCreditAllocation) InsertEligibility(context.Context, model.CreditEligibilityEntry) (model.CreditEligibilityEntry, error) {
	return model.CreditEligibilityEntry{}, cerr.InternalError(errors.New("unused"))
}

func (g *grantingCreditAllocation) InsertEligibilityMobile(context.Context, model.CreditEligibilityMobile) error {
	return cerr.InternalError(errors.New("unused"))
}

func (g *grantingCreditAllocation) ListEligibility(context.Context, *string, *uuid.UUID, int) ([]model.CreditEligibilityEntry, error) {
	return nil, nil
}

func (g *grantingCreditAllocation) ListEligibilityMobiles(context.Context, uuid.UUID) ([]model.CreditEligibilityMobile, error) {
	return nil, nil
}

func (g *grantingCreditAllocation) BindEligibility(context.Context, uuid.UUID, model.CreditOwnerID) error {
	return nil
}

func (g *grantingCreditAllocation) GetMembership(context.Context, uuid.UUID) (model.CreditGroupMembership, error) {
	return model.CreditGroupMembership{}, cerr.ResourceNotFound(errors.New("missing membership"))
}

func (g *grantingCreditAllocation) PutMembership(context.Context, model.CreditGroupMembership) (model.CreditGroupMembership, error) {
	return model.CreditGroupMembership{}, cerr.InternalError(errors.New("unused"))
}

func (g *grantingCreditAllocation) GetRule(
	_ context.Context,
	scope model.CreditAllocationScope,
	_ *string,
	entry *uuid.UUID,
	kind model.CreditKind,
	currency model.Currency,
) (model.CreditAllocationRule, error) {
	if scope == model.IndividualCreditAllocationScope && entry != nil && *entry == g.entryID {
		return model.CreditAllocationRule{
			ID:               g.ruleID,
			Scope:            model.IndividualCreditAllocationScope,
			EligibilityID:    &g.entryID,
			CreditKind:       kind,
			Currency:         currency,
			Strategy:         model.FixedLimitCreditAllocationStrategy,
			FixedAmountMinor: g.amount,
			Version:          1,
		}, nil
	}

	if scope == model.DefaultCreditAllocationScope {
		return model.CreditAllocationRule{
			ID:         model.CreditAllocationRuleID(uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")),
			Scope:      model.DefaultCreditAllocationScope,
			CreditKind: kind,
			Currency:   currency,
			Strategy:   model.DisabledCreditAllocationStrategy,
			Version:    1,
		}, nil
	}

	return model.CreditAllocationRule{}, cerr.ResourceNotFound(errors.New("missing rule"))
}

func (g *grantingCreditAllocation) PutRule(context.Context, model.CreditAllocationRule) (model.CreditAllocationRule, error) {
	return model.CreditAllocationRule{}, cerr.InternalError(errors.New("unused"))
}

func (g *grantingCreditAllocation) ListRules(context.Context) ([]model.CreditAllocationRule, error) {
	return nil, nil
}

func (g *grantingCreditAllocation) GetOpenRequest(context.Context, model.CreditOwnerID, model.CreditKind, model.Currency) (model.CreditRequest, error) {
	return model.CreditRequest{}, cerr.ResourceNotFound(errors.New("no open request"))
}

func (g *grantingCreditAllocation) GetLatestRequest(context.Context, model.CustomerID, model.CreditKind, model.Currency) (model.CreditRequest, error) {
	return model.CreditRequest{}, cerr.ResourceNotFound(errors.New("no request"))
}

func (g *grantingCreditAllocation) GetRequest(_ context.Context, id model.CreditRequestID, _ bool) (model.CreditRequest, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.request == nil || g.request.ID != id {
		return model.CreditRequest{}, cerr.ResourceNotFound(errors.New("no request"))
	}

	return *g.request, nil
}

func (g *grantingCreditAllocation) InsertRequest(_ context.Context, r model.CreditRequest) (model.CreditRequest, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	r.ID = g.requestID
	r.Status = model.PendingCreditRequestStatus
	r.UpdatedAt = r.RequestedAt
	copied := r
	g.request = &copied
	return r, nil
}

func (g *grantingCreditAllocation) QueueIdentityRecheck(context.Context, model.CreditRequestID, time.Time) error {
	return nil
}

func (g *grantingCreditAllocation) ClaimRequest(_ context.Context, id model.CreditRequestID, now, until time.Time) (model.CreditRequest, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.request == nil || g.request.ID != id {
		return model.CreditRequest{}, cerr.ResourceNotFound(errors.New("nothing to claim"))
	}

	token := uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
	g.request.Status = model.ProcessingCreditRequestStatus
	g.request.LeaseToken = &token
	g.request.LeaseUntil = &until
	g.request.UpdatedAt = now
	return *g.request, nil
}

func (g *grantingCreditAllocation) QueuePublicRequests(context.Context, time.Time, int) error {
	return nil
}

func (g *grantingCreditAllocation) ListDueRequests(context.Context, time.Time, int) ([]model.CreditRequest, error) {
	return nil, nil
}

func (g *grantingCreditAllocation) FinishRequest(
	_ context.Context,
	r model.CreditRequest,
	status model.CreditRequestStatus,
	reason string,
	decision *model.DecisionID,
	rule *model.CreditAllocationRuleID,
	next *time.Time,
	now time.Time,
) (model.CreditRequest, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.request == nil || g.request.ID != r.ID || g.request.LeaseToken == nil || r.LeaseToken == nil || *g.request.LeaseToken != *r.LeaseToken {
		return model.CreditRequest{}, cerr.ResourceNotFound(errors.New("lease mismatch"))
	}

	g.request.Status = status
	g.request.ReasonCode = reason
	g.request.DecisionID = decision
	g.request.AllocationRuleID = rule
	g.request.NextAttemptAt = next
	g.request.LeaseToken = nil
	g.request.LeaseUntil = nil
	g.request.UpdatedAt = now
	if status == model.ApprovedCreditRequestStatus || status == model.RejectedCreditRequestStatus || status == model.CanceledCreditRequestStatus {
		g.request.CompletedAt = &now
	}

	return *g.request, nil
}

func (g *grantingCreditAllocation) QueueEligibility(context.Context, *uuid.UUID, *string, *model.CreditKind, *model.Currency, time.Time) error {
	return nil
}

func (g *grantingCreditAllocation) ListPendingRequests(context.Context, model.CreditKind, time.Time, int) ([]model.CreditRequest, error) {
	return nil, nil
}

func TestAssignInitialLimit_FixedIndividualRule_GrantsMonthlyLimit(t *testing.T) {
	t.Parallel()

	ctx := testRequestContext()
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	externalCustomerID := model.ExternalCustomerID(uuid.New())
	customerID := model.CustomerID(uuid.New())
	profile := sampleActiveProfile(customerID, externalCustomerID, now)
	decisionID := model.DecisionID(uuid.MustParse("cccccccc-cccc-cccc-cccc-cccccccccccc"))
	accountID := model.CreditAccountID(uuid.MustParse("dddddddd-dddd-dddd-dddd-dddddddddddd"))
	amount := uint64(30_000_000)

	allocation := &grantingCreditAllocation{
		entryID:   uuid.MustParse("eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee"),
		ruleID:    model.CreditAllocationRuleID(uuid.MustParse("ffffffff-ffff-ffff-ffff-ffffffffffff")),
		requestID: model.CreditRequestID(uuid.MustParse("99999999-9999-9999-9999-999999999999")),
		amount:    amount,
	}

	installmentPolicy := initialcreditlimitv1.New()
	monthlyPolicy := initialcreditlimitv3.New()
	registry, err := initialcreditlimit.NewRegistry(installmentPolicy, monthlyPolicy)
	require.NoError(t, err)

	pool := mock.NewPool(t)
	conn := mock.NewConn(t)
	tx := mock.NewTx(t)
	customerProfile := mock.NewCustomerProfile(t)
	customerProfileTx := mock.NewCustomerProfileTxQueryer(t)
	creditAccount := mock.NewCreditAccount(t)
	creditAccountTx := mock.NewCreditAccountProductTxQueryer(t)
	decision := mock.NewDecision(t)
	decisionTx := mock.NewDecisionTxQueryer(t)
	outbox := mock.NewOutbox(t)
	outboxTx := mock.NewOutboxTxQueryer(t)
	policyAssignment := mock.NewPolicyAssignment(t)
	idGenerator := mock.NewIdentifierGenerator(t)
	customerResolver := mock.NewCustomerResolver(t)
	clk := adapterclock.NewFixed(now, time.UTC)

	expectTx := func() {
		pool.EXPECT().Conn(ctx, testifymock.Anything).RunAndReturn(
			func(ctx context.Context, handler repo.ConnHandler) error {
				return handler(ctx, conn)
			},
		).Once()
		conn.EXPECT().Tx(ctx, testifymock.Anything).RunAndReturn(
			func(ctx context.Context, handler repo.TxHandler) error {
				return handler(ctx, tx)
			},
		).Once()
	}
	expectTx()
	expectTx()
	expectTx()

	customerResolver.EXPECT().ResolveByMobile(
		ctx,
		extcustomer.ResolveByMobileInput{MobileE164: assignTestMobileE164},
	).Return(extcustomer.ResolveByMobileOutput{
		ExternalCustomerID: externalCustomerID,
		Status:             extcustomer.ActiveStatus,
		NationalCode:       "0012345678",
	}, nil).Times(2)

	customerProfile.EXPECT().Tx(tx).Return(customerProfileTx).Times(2)
	customerProfileTx.EXPECT().EnsureExists(
		ctx,
		testifymock.Anything,
		externalCustomerID,
		model.IRJurisdiction,
		model.ActiveCustomerStatus,
		now,
		now,
		"0012345678",
	).Return(profile, true, nil).Once()
	customerProfileTx.EXPECT().LockByID(ctx, customerID).Return(profile, nil).Once()

	creditAccount.EXPECT().Tx(tx).Return(creditAccountTx).Once()
	creditAccountTx.EXPECT().ExpireDueByCustomerIDCurrencyAndKind(
		ctx, customerID, model.IRRCurrency, model.PayNextMonthCreditKind, now,
	).Return(nil).Once()
	creditAccountTx.EXPECT().LockByCustomerIDCurrencyAndKind(
		ctx, customerID, model.IRRCurrency, model.PayNextMonthCreditKind,
	).Return(model.CreditAccount{}, cerr.ResourceNotFound(errors.New("missing"))).Once()

	var (
		nilActorID        *uuid.UUID
		nilPriorDecision  *model.DecisionID
		nilReviewableUntil *time.Time
		nilExpiresAt      *time.Time
	)
	decision.EXPECT().Tx(tx).Return(decisionTx).Once()
	decisionTx.EXPECT().Create(
		ctx,
		model.CreditLimitDecisionType,
		model.CustomerProfileDecisionTargetType,
		customerID.UUID(),
		model.LimitSetDecisionOutcome,
		[]string{"FIXED_CREDIT_ALLOCATION_APPROVED"},
		monthlyPolicy.Ref().ID,
		monthlyPolicy.Ref().Version,
		model.IRJurisdiction,
		model.SystemDecisionActorType,
		nilActorID,
		nilPriorDecision,
		nilReviewableUntil,
		nilExpiresAt,
		now,
		now,
		"credit-request:"+allocation.requestID.String(),
		testifymock.AnythingOfType("string"),
	).Return(model.Decision{ID: decisionID}, nil).Once()

	expires := now.Add(monthlyPolicy.Terms().ValidFor)
	createdAccount := model.CreditAccount{
		ID:                  accountID,
		CustomerID:          customerID,
		Currency:            model.IRRCurrency,
		CreditKind:          model.PayNextMonthCreditKind,
		Status:              model.ActiveCreditAccountStatus,
		LimitAmountMinor:    amount,
		LastLimitDecisionID: &decisionID,
		EffectiveFrom:       now,
		ExpiresAt:           &expires,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	creditAccountTx.EXPECT().CreateWithLimitForKind(
		ctx,
		customerID,
		model.IRRCurrency,
		model.PayNextMonthCreditKind,
		amount,
		decisionID,
		now,
		&expires,
		now,
		now,
	).Return(createdAccount, nil).Once()
	creditAccountTx.EXPECT().AppendTransaction(
		ctx,
		accountID,
		model.LimitSetCreditTransactionType,
		amount,
		model.IRRCurrency,
		model.CreditAssessmentCreditTransactionReason,
		model.DecisionCreditTransactionReferenceType,
		decisionID.UUID(),
		now,
		now,
	).Return(model.CreditTransaction{}, nil).Once()

	outbox.EXPECT().Tx(tx).Return(outboxTx).Once()
	idGenerator.EXPECT().Generate().Return("evt-grant").Once()
	outboxTx.EXPECT().Create(ctx, testifymock.Anything).Return(model.OutboxMessage{}, nil).Once()

	uc, err := customercredituc.New(
		pool,
		customerProfile,
		creditAccount,
		decision,
		mock.NewCustomerRestriction(t),
		mock.NewInstallment(t),
		mock.NewDefaultEpisode(t),
		outbox,
		clk,
		sha256fingerprint.New(),
		mock.NewCreditAssessmentClient(t),
		customerResolver,
		policyAssignment,
		registry,
		installmentPolicy.Ref(),
		"bnpl.audit.events.v1",
		idGenerator,
		collectionsv2.New(time.UTC),
		newTestObserver(t),
		absentTrustedMobile{},
		noExistingSharedCredit{},
		allocation,
		initialcreditcatalog.MonthlyRef(),
		mock.NewLogger(t),
	)
	require.NoError(t, err)

	output, err := uc.AssignInitialLimit(ctx, customercredituc.AssignInitialLimitInput{
		SubjectID:  model.SubjectID(uuid.New()),
		MobileE164: assignTestMobileE164,
		CreditKind: model.PayNextMonthCreditKind,
	})
	require.NoError(t, err)
	require.Equal(t, customercredituc.AssignInitialLimitResultCreated, output.Result)
	require.True(t, output.RequestCreated)
	require.Equal(t, decisionID, output.Decision.ID)
	require.NotNil(t, output.CreditLimit)
	require.Equal(t, amount, output.CreditLimit.LimitAmountMinor)
	require.Equal(t, model.PayNextMonthCreditKind, output.CreditLimit.CreditKind)
	require.NotNil(t, output.CreditRequest)
	require.Equal(t, model.ApprovedCreditRequestStatus, output.CreditRequest.Status)
	require.Equal(t, allocation.requestID, output.CreditRequest.ID)
}
