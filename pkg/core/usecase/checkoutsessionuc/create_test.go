package checkoutsessionuc

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/event/checkoutlifecycle"
	extcustomer "github.com/parstis/bnpl-backend/pkg/core/external/customer"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

const (
	testAttemptKey         = "attempt-1"
	merchantOrderConflict  = "error.merchant_order_id_conflicts_with_an_existing_checkout_session"
	testReturnDestination  = "storefront"
	testRequestedAmount    = uint64(150000000)
	testMerchantOrderID    = "order-1"
	testCustomerReference  = "cust-1"
	testMerchantStateValue = "cart-42"
)

func (h *harness) partnerRequest(mods ...func(*CheckoutSessionRequest)) CreatePartnerInput {
	request := CheckoutSessionRequest{
		MerchantID:           h.merchantID,
		OnlineStoreID:        h.storeID,
		MerchantOrderID:      testMerchantOrderID,
		IdempotencyKey:       testAttemptKey,
		CustomerReference:    testCustomerReference,
		ReturnDestination:    testReturnDestination,
		MerchantState:        testMerchantStateValue,
		RequestedAmountMinor: testRequestedAmount,
		Currency:             model.IRRCurrency,
	}

	for _, mod := range mods {
		mod(&request)
	}

	return CreatePartnerInput{
		CheckoutSessionRequest: request,
		CustomerPhone:          "+989121234567",
	}
}

func (h *harness) expectActiveMerchant() {
	h.merchantTx.EXPECT().
		GetByMerchantID(h.ctx, h.merchantID).
		Return(model.MerchantProfile{
			ID:                     h.merchantPID,
			MerchantID:             h.merchantID,
			Status:                 model.ActiveMerchantStatus,
			Jurisdiction:           model.IRJurisdiction,
			OfferPreparationMode:   model.OfferPreparationOnCreate,
		}, nil).Once()
}

func (h *harness) expectKeyNotFound() {
	h.sessionTx.EXPECT().
		LockByMerchantIdempotencyKey(h.ctx, h.merchantPID, testAttemptKey).
		Return(model.CheckoutSession{}, cerr.ResourceNotFound(errors.New("no attempt for key"))).Once()
}

// expectNoExistingSession expects both idempotency-key and order lookups to miss,
// so create proceeds to the new-origination path.
func (h *harness) expectNoExistingSession() {
	h.expectKeyNotFound()
	h.sessionTx.EXPECT().
		LockByMerchantOrderID(h.ctx, h.merchantPID, testMerchantOrderID).
		Return(model.CheckoutSession{}, cerr.ResourceNotFound(errors.New("no order attempt"))).Once()
}

// expectNewAttemptCreated expects the storefront resolution and insert for a
// brand-new checkout attempt and returns the inserted session.
func (h *harness) expectNewAttemptCreated() model.CheckoutSession {
	branchID := model.MerchantBranchProfileID(uuid.New())

	h.branchTx.EXPECT().
		LockByOnlineStoreID(h.ctx, h.storeID).
		Return(model.MerchantBranchProfile{ID: branchID, MerchantID: h.merchantID}, nil).Once()
	h.sessionTx.EXPECT().
		GetReturnDestination(h.ctx, h.merchantID, testReturnDestination).
		Return(model.MerchantReturnDestination{
			MerchantID:      h.merchantID,
			DestinationName: testReturnDestination,
			ReturnURL:       "https://shop.example/return",
			Enabled:         true,
		}, nil).Once()

	created := h.session(
		func(s *model.CheckoutSession) {
			s.ID = model.CheckoutSessionID(uuid.New())
			s.IdempotencyKey = testAttemptKey
			s.CustomerID = nil
			s.DecisionID = nil
		},
	)

	phoneCiphertext := "sb1.+989121234567"
	phoneKeyID := "test"
	created.PublicToken = h.publicToken
	created.Origin = model.ExternalPartnerCheckoutSessionOrigin
	created.CustomerPhoneCiphertext = &phoneCiphertext
	created.CustomerPhoneKeyID = &phoneKeyID
	created.OfferPreparationMode = model.OfferPreparationOnCreate

	h.sessionTx.EXPECT().
		Create(
			h.ctx,
			h.merchantPID,
			branchID,
			h.storeID,
			testMerchantOrderID,
			testAttemptKey,
			testifymock.AnythingOfType("model.CheckoutSessionToken"),
			model.ExternalPartnerCheckoutSessionOrigin,
			testCustomerReference,
			testifymock.AnythingOfType("*string"),
			testifymock.AnythingOfType("*string"),
			model.OfferPreparationOnCreate,
			(*model.CustomerID)(nil),
			testReturnDestination,
			"https://shop.example/return",
			testMerchantStateValue,
			testRequestedAmount,
			model.IRRCurrency,
			model.OpenCheckoutSessionStatus,
			h.now.Add(h.uc.ttl),
			h.now,
			h.now,
		).
		Return(created, nil).Once()

	return created
}

func TestCreatePartner_SameIdempotencyKeyReplaysExistingSession(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(1, 1)
	h.expectActiveMerchant()

	existing := h.session(func(s *model.CheckoutSession) { s.IdempotencyKey = testAttemptKey })

	// Replay never inserts: Create is not expected (strict mock).
	h.sessionTx.EXPECT().
		LockByMerchantIdempotencyKey(h.ctx, h.merchantPID, testAttemptKey).
		Return(existing, nil).Once()
	h.expectPreparedExternalOfferReload(existing)

	output, err := h.uc.CreatePartner(h.ctx, h.partnerRequest())
	require.NoError(t, err)
	require.Equal(t, CreateResultAlreadyExists, output.Result)
	require.Equal(t, existing.ID, output.Session.ID)
	require.Equal(t, "https://bnpl.example/checkout/"+existing.PublicToken.String(), output.CheckoutURL)
	require.Empty(t, h.notificationCalls)
	require.Empty(t, h.identityCalls)
}

func TestCreatePartner_SameIdempotencyKeyDifferentPayloadConflicts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mod  func(*CheckoutSessionRequest)
	}{
		{name: "amount", mod: func(r *CheckoutSessionRequest) { r.RequestedAmountMinor++ }},
		{name: "customer reference", mod: func(r *CheckoutSessionRequest) { r.CustomerReference = "cust-2" }},
		{name: "merchant state", mod: func(r *CheckoutSessionRequest) { r.MerchantState = "cart-43" }},
		{name: "return destination", mod: func(r *CheckoutSessionRequest) { r.ReturnDestination = "other" }},
		{name: "online store", mod: func(r *CheckoutSessionRequest) { r.OnlineStoreID = model.OnlineStoreID(uuid.New()) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)
			h.expectPool(0, 1)
			h.expectActiveMerchant()

			existing := h.session(func(s *model.CheckoutSession) { s.IdempotencyKey = testAttemptKey })
			h.sessionTx.EXPECT().
				LockByMerchantIdempotencyKey(h.ctx, h.merchantPID, testAttemptKey).
				Return(existing, nil).Once()

			// The stored session is never mutated and nothing is inserted.
			_, err := h.uc.CreatePartner(h.ctx, h.partnerRequest(tt.mod))
			requireCerr(t, err, cerr.CodeIdempotencyConflict, merchantOrderConflict)
		})
	}
}

func TestCreatePartner_NewAttemptAllowedAfterPreviousAttemptTerminalizes(t *testing.T) {
	t.Parallel()

	terminal := []model.CheckoutSessionStatus{
		model.CanceledCheckoutSessionStatus,
		model.ExpiredCheckoutSessionStatus,
		model.UnavailableCheckoutSessionStatus,
	}

	for _, status := range terminal {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)
			h.expectPool(0, 3)
			h.expectActiveMerchant()
			h.expectKeyNotFound()

			previous := h.session(withStatus(status), func(s *model.CheckoutSession) { s.IdempotencyKey = "attempt-0" })
			h.sessionTx.EXPECT().
				LockByMerchantOrderID(h.ctx, h.merchantPID, testMerchantOrderID).
				Return(previous, nil).Once()

			created := h.expectNewAttemptCreated()
			unbound := created
			bound := created
			bound.CustomerID = &h.customerID
			linked := bound
			linked.DecisionID = &h.decisionID
			h.expectExternalOfferPreparation(unbound, bound, linked)

			output, err := h.uc.CreatePartner(h.ctx, h.partnerRequest())
			require.NoError(t, err)
			require.Equal(t, CreateResultCreated, output.Result)
			require.Equal(t, created.ID, output.Session.ID)
			require.NotEqual(t, previous.ID, output.Session.ID)
		})
	}
}

func TestCreatePartner_NewAttemptRejectedWhenOrderAlreadyCompleted(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)
	h.expectActiveMerchant()
	h.expectKeyNotFound()

	completed := h.session(
		withStatus(model.CompletedCheckoutSessionStatus),
		withContract(h.contractID),
		func(s *model.CheckoutSession) { s.IdempotencyKey = "attempt-0" },
	)
	h.sessionTx.EXPECT().
		LockByMerchantOrderID(h.ctx, h.merchantPID, testMerchantOrderID).
		Return(completed, nil).Once()

	// Storefront resolution and Create are not expected (strict mocks).
	_, err := h.uc.CreatePartner(h.ctx, h.partnerRequest())
	requireCerr(t, err, cerr.CodeIdempotencyConflict, merchantOrderConflict)
}

func TestCreatePartner_LegacyActiveAttemptForSameOrderIsReplayed(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(1, 1)
	h.expectActiveMerchant()
	h.expectKeyNotFound()

	// An active attempt created under the legacy key (the order id) is still the
	// current attempt when the terms match.
	legacy := h.session(func(s *model.CheckoutSession) { s.IdempotencyKey = testMerchantOrderID })
	h.sessionTx.EXPECT().
		LockByMerchantOrderID(h.ctx, h.merchantPID, testMerchantOrderID).
		Return(legacy, nil).Once()
	h.expectPreparedExternalOfferReload(legacy)

	output, err := h.uc.CreatePartner(h.ctx, h.partnerRequest())
	require.NoError(t, err)
	require.Equal(t, CreateResultAlreadyExists, output.Result)
	require.Equal(t, legacy.ID, output.Session.ID)
	require.Empty(t, h.notificationCalls)
}

func TestCreatePartner_ActiveAttemptWithDifferentTermsConflicts(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)
	h.expectActiveMerchant()
	h.expectKeyNotFound()

	active := h.session(
		withStatus(model.ProcessingCheckoutSessionStatus),
		func(s *model.CheckoutSession) { s.IdempotencyKey = "attempt-0" },
	)
	h.sessionTx.EXPECT().
		LockByMerchantOrderID(h.ctx, h.merchantPID, testMerchantOrderID).
		Return(active, nil).Once()

	_, err := h.uc.CreatePartner(h.ctx, h.partnerRequest(func(r *CheckoutSessionRequest) {
		r.RequestedAmountMinor = 1
	}))
	requireCerr(t, err, cerr.CodeIdempotencyConflict, merchantOrderConflict)
}

func TestCreatePartner_IdempotentReplayDifferentAuthScopeConflicts(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)
	h.expectActiveMerchant()

	// Session snapshotted a different payment scope than the merchant's
	// current provisioned scope — immutable scope matching must conflict.
	existing := h.session(func(s *model.CheckoutSession) {
		s.OfferPreparationMode = model.OfferPreparationOnStart
	})
	h.sessionTx.EXPECT().
		LockByMerchantIdempotencyKey(h.ctx, h.merchantPID, testAttemptKey).
		Return(existing, nil).Once()

	_, err := h.uc.CreatePartner(h.ctx, h.partnerRequest())
	requireCerr(t, err, cerr.CodeIdempotencyConflict, merchantOrderConflict)
}

func TestCreatePartner_SnapshotsOfferPreparationModeOntoSession(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 3)
	h.expectActiveMerchant()
	h.expectNoExistingSession()
	created := h.expectNewAttemptCreated()
	unbound := created
	bound := created
	bound.CustomerID = &h.customerID
	linked := bound
	linked.DecisionID = &h.decisionID
	h.expectExternalOfferPreparation(unbound, bound, linked)

	output, err := h.uc.CreatePartner(h.ctx, h.partnerRequest())
	require.NoError(t, err)
	require.Equal(t, CreateResultCreated, output.Result)
	require.Equal(t, model.OfferPreparationOnCreate, output.Session.EffectiveOfferPreparationMode())
}


func TestCreatePartner_IdentityNotFoundTerminalizesUnavailable(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 2)
	h.expectActiveMerchant()
	h.expectKeyNotFound()

	previous := h.session(
		withStatus(model.ExpiredCheckoutSessionStatus),
		func(s *model.CheckoutSession) { s.IdempotencyKey = "attempt-0" },
	)
	h.sessionTx.EXPECT().
		LockByMerchantOrderID(h.ctx, h.merchantPID, testMerchantOrderID).
		Return(previous, nil).Once()

	created := h.expectNewAttemptCreated()
	unavailable := created
	unavailable.Status = model.UnavailableCheckoutSessionStatus

	h.identityErr = cerr.ResourceNotFound(errors.New("identity subject not found"))
	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(created, nil).Once()
	h.sessionTx.EXPECT().
		UpdateStatus(h.ctx, created.ID, model.UnavailableCheckoutSessionStatus, h.now).
		Return(unavailable, nil).Once()
	h.expectLifecycleEvent(unavailable, checkoutlifecycle.EventTypeUnavailable)

	output, err := h.uc.CreatePartner(h.ctx, h.partnerRequest())
	require.NoError(t, err)
	require.Equal(t, CreateResultCreated, output.Result)
	require.Equal(t, model.UnavailableCheckoutSessionStatus, output.Session.Status)
	require.Len(t, h.identityCalls, 1)
	require.Empty(t, h.decisions.calls)
	require.Empty(t, h.notificationCalls)
}

func TestCreatePartner_CustomerSuspendedTerminalizesUnavailable(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 2)
	h.expectActiveMerchant()
	h.expectKeyNotFound()

	previous := h.session(
		withStatus(model.ExpiredCheckoutSessionStatus),
		func(s *model.CheckoutSession) { s.IdempotencyKey = "attempt-0" },
	)
	h.sessionTx.EXPECT().
		LockByMerchantOrderID(h.ctx, h.merchantPID, testMerchantOrderID).
		Return(previous, nil).Once()

	created := h.expectNewAttemptCreated()
	unavailable := created
	unavailable.Status = model.UnavailableCheckoutSessionStatus

	h.identityOutput = extcustomer.ResolveByMobileOutput{
		ExternalCustomerID: h.profileID,
		Status:             extcustomer.SuspendedStatus,
	}
	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(created, nil).Once()
	h.sessionTx.EXPECT().
		UpdateStatus(h.ctx, created.ID, model.UnavailableCheckoutSessionStatus, h.now).
		Return(unavailable, nil).Once()
	h.expectLifecycleEvent(unavailable, checkoutlifecycle.EventTypeUnavailable)

	output, err := h.uc.CreatePartner(h.ctx, h.partnerRequest())
	require.NoError(t, err)
	require.Equal(t, model.UnavailableCheckoutSessionStatus, output.Session.Status)
	require.Len(t, h.identityCalls, 1)
	require.Empty(t, h.decisions.calls)
	require.Empty(t, h.notificationCalls)
}

func TestCreatePartner_UnsupportedCustomerStatusFailsClosed(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)
	h.expectActiveMerchant()
	h.expectKeyNotFound()

	previous := h.session(
		withStatus(model.ExpiredCheckoutSessionStatus),
		func(s *model.CheckoutSession) { s.IdempotencyKey = "attempt-0" },
	)
	h.sessionTx.EXPECT().
		LockByMerchantOrderID(h.ctx, h.merchantPID, testMerchantOrderID).
		Return(previous, nil).Once()

	_ = h.expectNewAttemptCreated()
	h.identityOutput = extcustomer.ResolveByMobileOutput{
		ExternalCustomerID: h.profileID,
		Status:             extcustomer.Status("unspecified"),
	}

	_, err := h.uc.CreatePartner(h.ctx, h.partnerRequest())
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeInternalError))
	require.Len(t, h.identityCalls, 1)
	require.Empty(t, h.decisions.calls)
	require.Empty(t, h.notificationCalls)
}

func TestCreatePartner_PreparesOfferWithoutSendingOTP(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 3)
	h.expectActiveMerchant()
	h.expectKeyNotFound()

	previous := h.session(
		withStatus(model.ExpiredCheckoutSessionStatus),
		func(s *model.CheckoutSession) { s.IdempotencyKey = "attempt-0" },
	)
	h.sessionTx.EXPECT().
		LockByMerchantOrderID(h.ctx, h.merchantPID, testMerchantOrderID).
		Return(previous, nil).Once()

	created := h.expectNewAttemptCreated()
	unbound := created
	bound := created
	bound.CustomerID = &h.customerID
	linked := bound
	linked.DecisionID = &h.decisionID
	h.expectExternalOfferPreparation(unbound, bound, linked)

	output, err := h.uc.CreatePartner(h.ctx, h.partnerRequest())
	require.NoError(t, err)
	require.Equal(t, CreateResultCreated, output.Result)
	require.NotNil(t, output.Session.CustomerID)
	require.NotNil(t, output.Session.DecisionID)
	require.Len(t, h.identityCalls, 1)
	require.Len(t, h.decisions.calls, 1)
	require.Empty(t, h.notificationCalls)
}
