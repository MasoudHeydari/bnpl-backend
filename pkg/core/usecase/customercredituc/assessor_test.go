package customercredituc_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	adapterclock "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	sha256fingerprint "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/fingerprint/sha256"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/external/creditassessment"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	corepolicy "github.com/parstis/bnpl-backend/pkg/core/policy"
	"github.com/parstis/bnpl-backend/pkg/core/policy/initialcreditlimit"
	initialcreditlimitv1 "github.com/parstis/bnpl-backend/pkg/core/policy/initialcreditlimit/v1"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/customercredituc"
)

func newAssessUseCase(
	t *testing.T,
	client *mock.CreditAssessmentClient,
	pool *mock.Pool,
	policyAssignment *mock.PolicyAssignment,
	registry *initialcreditlimit.Registry,
	defaultRef corepolicy.Ref,
) *customercredituc.UseCase {
	t.Helper()

	uc, err := customercredituc.New(
		pool,
		mock.NewCustomerProfile(t),
		mock.NewCreditAccount(t),
		mock.NewDecision(t),
		mock.NewOutbox(t),
		adapterclock.NewFixed(time.Date(
			2026,
			7,
			22,
			12,
			0,
			0,
			0,
			time.UTC,
		), time.UTC),
		sha256fingerprint.New(),
		client,
		policyAssignment,
		registry,
		defaultRef,
		"bnpl.audit.events.v1",
		mock.NewIdentifierGenerator(t),
	)
	require.NoError(t, err)

	return uc
}

func expectAssessDefaultPolicyAssignment(
	t *testing.T,
	pool *mock.Pool,
	policyAssignment *mock.PolicyAssignment,
	times int,
) {
	t.Helper()

	conn := mock.NewConn(t)
	cq := mock.NewPolicyAssignmentConnQueryer(t)

	pool.EXPECT().Conn(testifymock.Anything, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.ConnHandler) error {
			return handler(ctx, conn)
		},
	).Times(times)
	policyAssignment.EXPECT().Conn(conn).Return(cq).Times(times)
	cq.EXPECT().GetActive(
		testifymock.Anything,
		model.CustomerPolicySubjectType,
		testifymock.Anything,
		model.InitialCreditLimitPolicyFamily,
		testifymock.Anything,
	).Return(model.PolicyAssignment{}, cerr.ResourceNotFound(errors.New("missing"))).Times(times)
}

func TestAssessInitialLimit_UsesInjectedPolicyRef(t *testing.T) {
	t.Parallel()

	ref := corepolicy.Ref{ID: "distinguishable_policy_id", Version: "v42"}
	terms := initialcreditlimit.Terms{
		DefaultAmountMinor: 1_000,
		MaxAmountMinor:     1_000,
		Currency:           model.IRRCurrency,
		ValidFor:           24 * time.Hour,
		ApprovalReasonCode: "DISTINGUISHABLE_REASON",
	}

	policy := mock.NewInitialCreditLimitPolicy(t)
	policy.EXPECT().Ref().Return(ref)
	policy.EXPECT().Terms().Return(terms).Once()

	registry, err := initialcreditlimit.NewRegistry(policy)
	require.NoError(t, err)

	pool := mock.NewPool(t)
	policyAssignment := mock.NewPolicyAssignment(t)
	expectAssessDefaultPolicyAssignment(t, pool, policyAssignment, 1)

	client := mock.NewCreditAssessmentClient(t)
	client.EXPECT().FetchSignals(testifymock.Anything, testifymock.Anything).
		Return(creditassessment.FetchSignalsResult{}, nil).Once()

	uc := newAssessUseCase(
		t,
		client,
		pool,
		policyAssignment,
		registry,
		ref,
	)

	result, err := uc.AssessInitialLimit(context.Background(), customercredituc.InitialLimitAssessmentInput{
		CustomerID:         model.CustomerID(uuid.New()),
		ExternalCustomerID: model.ExternalCustomerID(uuid.New()),
		Jurisdiction:       model.IRJurisdiction,
		RequestedAt: time.Date(
			2026,
			7,
			22,
			12,
			0,
			0,
			0,
			time.UTC,
		),
	})
	require.NoError(t, err)
	require.Equal(t, "distinguishable_policy_id", result.PolicyID)
	require.Equal(t, "v42", result.PolicyVersion)
	require.Equal(t, "DISTINGUISHABLE_REASON", result.ReasonCodes[0])
}

func TestAssessInitialLimit_DeterministicAgainstRealPolicy(t *testing.T) {
	t.Parallel()

	policy := initialcreditlimitv1.New()
	registry, err := initialcreditlimit.NewRegistry(policy)
	require.NoError(t, err)

	terms := policy.Terms()

	pool := mock.NewPool(t)
	policyAssignment := mock.NewPolicyAssignment(t)
	expectAssessDefaultPolicyAssignment(t, pool, policyAssignment, 2)

	client := mock.NewCreditAssessmentClient(t)
	client.EXPECT().FetchSignals(testifymock.Anything, testifymock.Anything).
		Return(creditassessment.FetchSignalsResult{}, nil).Twice()

	uc := newAssessUseCase(
		t,
		client,
		pool,
		policyAssignment,
		registry,
		policy.Ref(),
	)

	input := customercredituc.InitialLimitAssessmentInput{
		CustomerID:         model.CustomerID(uuid.New()),
		ExternalCustomerID: model.ExternalCustomerID(uuid.New()),
		Jurisdiction:       model.IRJurisdiction,
		RequestedAt: time.Date(
			2026,
			7,
			22,
			12,
			0,
			0,
			0,
			time.UTC,
		),
	}

	first, err := uc.AssessInitialLimit(context.Background(), input)
	require.NoError(t, err)

	second, err := uc.AssessInitialLimit(context.Background(), input)
	require.NoError(t, err)

	require.Equal(t, first, second)
	require.Equal(t, policy.Ref().ID, first.PolicyID)
	require.Equal(t, policy.Ref().Version, first.PolicyVersion)
	require.Equal(t, terms.DefaultAmountMinor, first.AmountMinor)
	require.Equal(t, terms.Currency, first.Currency)
}

func TestAssessInitialLimit_PropagatesClientError(t *testing.T) {
	t.Parallel()

	policy := initialcreditlimitv1.New()
	registry, err := initialcreditlimit.NewRegistry(policy)
	require.NoError(t, err)

	pool := mock.NewPool(t)
	policyAssignment := mock.NewPolicyAssignment(t)
	expectAssessDefaultPolicyAssignment(t, pool, policyAssignment, 1)

	client := mock.NewCreditAssessmentClient(t)
	client.EXPECT().FetchSignals(testifymock.Anything, testifymock.Anything).
		Return(creditassessment.FetchSignalsResult{}, errors.New("bureau unavailable")).Once()

	uc := newAssessUseCase(
		t,
		client,
		pool,
		policyAssignment,
		registry,
		policy.Ref(),
	)

	_, err = uc.AssessInitialLimit(context.Background(), customercredituc.InitialLimitAssessmentInput{
		CustomerID:         model.CustomerID(uuid.New()),
		ExternalCustomerID: model.ExternalCustomerID(uuid.New()),
		Jurisdiction:       model.IRJurisdiction,
		RequestedAt: time.Date(
			2026,
			7,
			22,
			12,
			0,
			0,
			0,
			time.UTC,
		),
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "bureau unavailable")
}
