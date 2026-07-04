package decisionuc_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	adapterclock "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	sha256fingerprint "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/fingerprint/sha256"
	metricsnoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/metrics/noop"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	checkoutcatalog "github.com/parstis/bnpl-backend/pkg/core/policy/checkout/catalog"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/decisionuc"
)

func TestRevalidateCheckoutOffer_StillHonorableWhenAvailableCreditDropsButTermsMatch(t *testing.T) {
	t.Parallel()

	h := newRevalidateHarness(t)
	h.wireProfilesAndCredit(150_000) // dropped from 200k snapshot; 100k offer still fits

	got, err := h.uc.RevalidateCheckoutOffer(h.ctx, decisionuc.RevalidateCheckoutOfferInput{
		DecisionID: h.decisionID,
	})
	require.NoError(t, err)
	require.True(t, got.StillHonorable)
}

func TestRevalidateCheckoutOffer_NotHonorableWhenFinancedTermsChange(t *testing.T) {
	t.Parallel()

	h := newRevalidateHarness(t)
	h.wireProfilesAndCredit(80_000) // forces financed 80 + upfront 20

	got, err := h.uc.RevalidateCheckoutOffer(h.ctx, decisionuc.RevalidateCheckoutOfferInput{
		DecisionID: h.decisionID,
	})
	require.NoError(t, err)
	require.False(t, got.StillHonorable)
}

func TestRevalidateCheckoutOffer_UsesPersistedPolicySnapshotNotActiveAssignment(t *testing.T) {
	t.Parallel()

	h := newRevalidateHarness(t)
	h.wireProfilesAndCredit(200_000)

	// Revalidation must not consult the active policy assignment table.
	h.assignment.AssertNotCalled(
		t,
		"Conn",
		testifymock.Anything,
	)

	got, err := h.uc.RevalidateCheckoutOffer(h.ctx, decisionuc.RevalidateCheckoutOfferInput{
		DecisionID: h.decisionID,
	})
	require.NoError(t, err)
	require.True(t, got.StillHonorable)
}

type revalidateHarness struct {
	t            *testing.T
	ctx          context.Context
	now          time.Time
	uc           *decisionuc.UseCase
	pool         *mock.Pool
	conn         *mock.Conn
	decision     *mock.Decision
	decisionCQ   *mock.DecisionConnQueryer
	allocation   *mock.CheckoutDecisionAllocation
	allocationCQ *mock.CheckoutDecisionAllocationConnQueryer
	customer     *mock.CustomerProfile
	customerCQ   *mock.CustomerProfileConnQueryer
	merchant     *mock.MerchantProfile
	merchantCQ   *mock.MerchantProfileConnQueryer
	branch       *mock.MerchantBranchProfile
	branchCQ     *mock.MerchantBranchProfileConnQueryer
	credit       *mock.CreditAccount
	creditCQ     *mock.CreditAccountConnQueryer
	assignment   *mock.PolicyAssignment
	decisionID   model.DecisionID
	customerID   model.CustomerID
	profileID    model.IdentitySubjectID
	merchantPID  model.MerchantProfileID
	merchantID   model.MerchantID
	branchPID    model.MerchantBranchProfileID
	expires      time.Time
}

func newRevalidateHarness(t *testing.T) *revalidateHarness {
	t.Helper()

	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	h := &revalidateHarness{
		t:            t,
		ctx:          context.Background(),
		now:          now,
		pool:         mock.NewPool(t),
		conn:         mock.NewConn(t),
		decision:     mock.NewDecision(t),
		decisionCQ:   mock.NewDecisionConnQueryer(t),
		allocation:   mock.NewCheckoutDecisionAllocation(t),
		allocationCQ: mock.NewCheckoutDecisionAllocationConnQueryer(t),
		customer:     mock.NewCustomerProfile(t),
		customerCQ:   mock.NewCustomerProfileConnQueryer(t),
		merchant:     mock.NewMerchantProfile(t),
		merchantCQ:   mock.NewMerchantProfileConnQueryer(t),
		branch:       mock.NewMerchantBranchProfile(t),
		branchCQ:     mock.NewMerchantBranchProfileConnQueryer(t),
		credit:       mock.NewCreditAccount(t),
		creditCQ:     mock.NewCreditAccountConnQueryer(t),
		assignment:   mock.NewPolicyAssignment(t),
		decisionID:   model.DecisionID(uuid.New()),
		customerID:   model.CustomerID(uuid.New()),
		profileID:    model.IdentitySubjectID(uuid.New()),
		merchantPID:  model.MerchantProfileID(uuid.New()),
		merchantID:   model.MerchantID(uuid.New()),
		branchPID:    model.MerchantBranchProfileID(uuid.New()),
		expires:      now.Add(15 * time.Minute),
	}

	installment := mock.NewInstallment(t)
	charge := mock.NewInstallmentCharge(t)
	outbox := mock.NewOutbox(t)
	idGen := mock.NewIdentifierGenerator(t)

	registry, err := checkoutcatalog.NewRegistry()
	require.NoError(t, err)

	uc, err := decisionuc.New(
		h.pool,
		h.decision,
		h.customer,
		h.credit,
		h.merchant,
		h.branch,
		h.allocation,
		h.assignment,
		installment,
		charge,
		outbox,
		adapterclock.NewFixed(now, time.UTC),
		sha256fingerprint.New(),
		registry,
		checkoutcatalog.DefaultRef(),
		"bnpl.audit.events.v1",
		idGen,
		mustTestJurisdictions(t),
		metricsnoop.New(),
	)
	require.NoError(t, err)
	h.uc = uc

	return h
}

func (h *revalidateHarness) wireProfilesAndCredit(limitMinor uint64) {
	h.t.Helper()

	h.pool.EXPECT().Conn(h.ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.ConnHandler) error {
			return handler(ctx, h.conn)
		},
	).Once()
	h.decision.EXPECT().Conn(h.conn).Return(h.decisionCQ).Once()
	h.allocation.EXPECT().Conn(h.conn).Return(h.allocationCQ).Once()
	h.customer.EXPECT().Conn(h.conn).Return(h.customerCQ).Once()
	h.merchant.EXPECT().Conn(h.conn).Return(h.merchantCQ).Once()
	h.branch.EXPECT().Conn(h.conn).Return(h.branchCQ).Once()
	h.credit.EXPECT().Conn(h.conn).Return(h.creditCQ).Once()

	decision := model.Decision{
		ID:            h.decisionID,
		Outcome:       model.ApprovedDecisionOutcome,
		PolicyID:      checkoutcatalog.DefaultRef().ID,
		PolicyVersion: checkoutcatalog.DefaultRef().Version,
		ExpiresAt:     &h.expires,
	}
	allocation := model.CheckoutDecisionAllocation{
		DecisionID:                  h.decisionID,
		CustomerID:                  h.customerID,
		MerchantProfileID:           h.merchantPID,
		MerchantBranchProfileID:     &h.branchPID,
		RequestedAmountMinor:        100_000,
		ApprovedFinancedAmountMinor: 100_000,
		UpfrontAmountMinor:          0,
		AvailableCreditMinor:        200_000,
		Currency:                    model.IRRCurrency,
		InstallmentCount:            4,
		ExpiresAt:                   h.expires,
	}
	customer := model.CustomerProfile{
		ID:                h.customerID,
		IdentitySubjectID: h.profileID,
		Status:            model.ActiveCustomerStatus,
		Jurisdiction:      model.IRJurisdiction,
	}
	merchant := model.MerchantProfile{
		ID:                h.merchantPID,
		MerchantID:        h.merchantID,
		Status:            model.ActiveMerchantStatus,
		Jurisdiction:      model.IRJurisdiction,
		Currencies:        []model.Currency{model.IRRCurrency},
		MaxAmountMinor:    2_000_000_000,
		MaxAmountCurrency: model.IRRCurrency,
		MaxTenureMonths:   12,
	}
	branch := model.MerchantBranchProfile{
		ID:         h.branchPID,
		MerchantID: h.merchantID,
		Status:     model.ActiveMerchantBranchStatus,
	}
	account := model.CreditAccount{
		CustomerID:          h.customerID,
		Currency:            model.IRRCurrency,
		LimitAmountMinor:    limitMinor,
		ExposureAmountMinor: 0,
		Status:              model.ActiveCreditAccountStatus,
	}

	h.decisionCQ.EXPECT().GetByID(h.ctx, h.decisionID).Return(decision, nil).Once()
	h.allocationCQ.EXPECT().GetByDecisionID(h.ctx, h.decisionID).Return(allocation, nil).Once()
	h.customerCQ.EXPECT().GetByID(h.ctx, h.customerID).Return(customer, nil).Once()
	h.merchantCQ.EXPECT().GetByID(h.ctx, h.merchantPID).Return(merchant, nil).Once()
	h.branchCQ.EXPECT().GetByID(h.ctx, h.branchPID).Return(branch, nil).Once()
	h.creditCQ.EXPECT().GetByCustomerIDAndCurrency(h.ctx, h.customerID, model.IRRCurrency).Return(account, nil).Once()
}
