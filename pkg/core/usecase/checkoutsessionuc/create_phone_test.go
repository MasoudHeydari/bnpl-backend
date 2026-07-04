package checkoutsessionuc

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/canonicalizer/phonenumber"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/domaincapability"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestCreatePartner_LocalIranianPhoneCanonicalizesBeforePersist(t *testing.T) {
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

	input := h.partnerRequest()
	input.CustomerPhone = "09121234567"

	output, err := h.uc.CreatePartner(h.ctx, input)
	require.NoError(t, err)
	require.Equal(t, CreateResultCreated, output.Result)
	require.Len(t, h.identityCalls, 1)
	require.Equal(t, "+989121234567", h.identityCalls[0].MobileE164)
	require.Empty(t, h.notificationCalls)
}

func TestCreatePartner_E164IranianPhoneSameCanonical(t *testing.T) {
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

	input := h.partnerRequest()
	input.CustomerPhone = "+989121234567"

	output, err := h.uc.CreatePartner(h.ctx, input)
	require.NoError(t, err)
	require.Equal(t, CreateResultCreated, output.Result)
	require.Equal(t, "+989121234567", h.identityCalls[0].MobileE164)
}

func TestCreatePartner_InvalidPhoneForMerchantJurisdiction(t *testing.T) {
	t.Parallel()

	for _, phone := range []string{
		"+14155552671",
		"+905551234567",
		" 09121234567",
		"+982112345678",
	} {
		t.Run(phone, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)
			h.expectPool(0, 1)
			h.expectActiveMerchant()
			h.expectNoExistingSession()

			input := h.partnerRequest()
			input.CustomerPhone = phone

			_, err := h.uc.CreatePartner(h.ctx, input)
			requireCerr(t, err, cerr.CodeInvalidArgument, "error.customer_phone_invalid")
			require.Empty(t, h.identityCalls)
			require.Empty(t, h.notificationCalls)
		})
	}
}

func TestCreatePartner_UnsupportedMerchantJurisdictionFailsClosed(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)
	h.merchantTx.EXPECT().
		GetByMerchantID(h.ctx, h.merchantID).
		Return(model.MerchantProfile{
			ID:               h.merchantPID,
			MerchantID:       h.merchantID,
			Status:           model.ActiveMerchantStatus,
			Jurisdiction:     model.Jurisdiction("TR"),
		}, nil).Once()
	h.expectNoExistingSession()

	_, err := h.uc.CreatePartner(h.ctx, h.partnerRequest())
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeInternalError))
	require.False(t, cerr.IsCode(err, cerr.CodeInvalidArgument))
	require.Empty(t, h.identityCalls)
	require.Empty(t, h.notificationCalls)
}

func TestCreatePartner_IdempotentReplayDifferentRawSameCanonical(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(2, 1)
	h.expectActiveMerchant()

	existing := h.session(func(s *model.CheckoutSession) { s.IdempotencyKey = testAttemptKey })
	h.sessionTx.EXPECT().
		LockByMerchantIdempotencyKey(h.ctx, h.merchantPID, testAttemptKey).
		Return(existing, nil).Once()
	h.expectPreparedExternalOfferReload(existing)

	input := h.partnerRequest()
	input.CustomerPhone = "09121234567"

	output, err := h.uc.CreatePartner(h.ctx, input)
	require.NoError(t, err)
	require.Equal(t, CreateResultAlreadyExists, output.Result)
	require.Equal(t, existing.ID, output.Session.ID)
	require.Empty(t, h.identityCalls)
}

func TestCreatePartner_IdempotentReplayReverseRawForms(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(2, 1)
	h.expectActiveMerchant()

	// Stored ciphertext is for +989121234567; replay with E.164 must also match.
	phoneCiphertext := "sb1.+989121234567"
	phoneKeyID := "test"
	existing := h.session(func(s *model.CheckoutSession) {
		s.IdempotencyKey = testAttemptKey
		s.CustomerPhoneCiphertext = &phoneCiphertext
		s.CustomerPhoneKeyID = &phoneKeyID
	})
	h.sessionTx.EXPECT().
		LockByMerchantIdempotencyKey(h.ctx, h.merchantPID, testAttemptKey).
		Return(existing, nil).Once()
	h.expectPreparedExternalOfferReload(existing)

	input := h.partnerRequest()
	input.CustomerPhone = "+989121234567"

	output, err := h.uc.CreatePartner(h.ctx, input)
	require.NoError(t, err)
	require.Equal(t, CreateResultAlreadyExists, output.Result)
}

func TestCreatePartner_ConflictRecoveryCanonicalizesBeforePhoneCompare(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	// Create race: Tx Create returns Conflict, Conn recovery, then 2 Conn prepare.
	h.expectPool(3, 1)
	h.expectActiveMerchant()
	h.expectKeyNotFound()

	previous := h.session(
		withStatus(model.ExpiredCheckoutSessionStatus),
		func(s *model.CheckoutSession) { s.IdempotencyKey = "attempt-0" },
	)
	h.sessionTx.EXPECT().
		LockByMerchantOrderID(h.ctx, h.merchantPID, testMerchantOrderID).
		Return(previous, nil).Once()

	branchID := model.MerchantBranchProfileID{}
	h.branchTx.EXPECT().
		LockByOnlineStoreID(h.ctx, h.storeID).
		Return(model.MerchantBranchProfile{ID: branchID, MerchantID: h.merchantID, OfferPreparationMode: model.OfferPreparationOnCreate}, nil).Once()
	h.sessionTx.EXPECT().
		GetReturnDestination(h.ctx, h.merchantID, testReturnDestination).
		Return(model.MerchantReturnDestination{
			MerchantID:      h.merchantID,
			DestinationName: testReturnDestination,
			ReturnURL:       "https://shop.example/return",
			Enabled:         true,
		}, nil).Once()
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
		Return(model.CheckoutSession{}, cerr.Conflict(errors.New("unique race"))).Once()

	existing := h.session(func(s *model.CheckoutSession) { s.IdempotencyKey = testAttemptKey })
	h.merchantCQ.EXPECT().
		GetByMerchantID(h.ctx, h.merchantID).
		Return(model.MerchantProfile{
			ID:               h.merchantPID,
			MerchantID:       h.merchantID,
			Status:           model.ActiveMerchantStatus,
			Jurisdiction:     model.IRJurisdiction,
		}, nil).Once()
	h.sessionCQ.EXPECT().
		GetByMerchantIdempotencyKey(h.ctx, h.merchantPID, testAttemptKey).
		Return(existing, nil).Once()
	h.expectPreparedExternalOfferReload(existing)

	input := h.partnerRequest()
	input.CustomerPhone = "09121234567"

	output, err := h.uc.CreatePartner(h.ctx, input)
	require.NoError(t, err)
	require.Equal(t, CreateResultAlreadyExists, output.Result)
	require.Equal(t, existing.ID, output.Session.ID)
}

func TestMapPhoneCanonicalizeError_UnsupportedIsInternal(t *testing.T) {
	t.Parallel()

	err := mapPhoneCanonicalizeError("op", phonenumber.ErrUnsupportedJurisdiction)
	require.True(t, cerr.IsCode(err, cerr.CodeInternalError))

	err = mapPhoneCanonicalizeError("op", phonenumber.ErrInvalid)
	requireCerr(t, err, cerr.CodeInvalidArgument, "error.customer_phone_invalid")

	err = mapPhoneCanonicalizeError("op", phonenumber.ErrRequired)
	requireCerr(t, err, cerr.CodeInvalidArgument, "error.customer_phone_required")
}

func TestMapPhoneCanonicalizeError_UnknownDoesNotLeakPhone(t *testing.T) {
	t.Parallel()

	raw := "+989121234567"
	err := mapPhoneCanonicalizeError("op", errors.New("provider failed for "+raw))
	require.True(t, cerr.IsCode(err, cerr.CodeInternalError))
	require.NotContains(t, err.Error(), raw)
	require.Contains(t, err.Error(), "phone canonicalizer failed")
}

func TestCreatePartner_StubUnsupportedJurisdiction(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.phoneCanonicalizeErr = phonenumber.ErrUnsupportedJurisdiction
	h.expectPool(0, 1)
	h.expectActiveMerchant()
	h.expectNoExistingSession()

	_, err := h.uc.CreatePartner(h.ctx, h.partnerRequest())
	require.True(t, cerr.IsCode(err, cerr.CodeInternalError))
}

func TestCreatePartner_UnknownCanonicalizerErrorDoesNotLeakPhone(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	leaky := "+989121234567"
	h.phoneCanonicalizeErr = errors.New("provider failed for " + leaky)
	h.expectPool(0, 1)
	h.expectActiveMerchant()
	h.expectNoExistingSession()

	_, err := h.uc.CreatePartner(h.ctx, h.partnerRequest())
	require.True(t, cerr.IsCode(err, cerr.CodeInternalError))
	require.NotContains(t, err.Error(), leaky)
	require.Empty(t, h.identityCalls)
	require.Empty(t, h.notificationCalls)
}

func TestCreatePartner_CustomerPhoneOverMaxLengthRejectsBeforeDB(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	// No pool expectations: validation must fail before repository work.

	input := h.partnerRequest()
	input.CustomerPhone = strings.Repeat("0", model.MaxCustomerPhoneLength+1)

	_, err := h.uc.CreatePartner(h.ctx, input)
	requireCerr(t, err, cerr.CodeInvalidArgument, "error.customer_phone_invalid")
	require.Empty(t, h.identityCalls)
	require.Empty(t, h.notificationCalls)
}

func TestCreatePartner_CustomerPhoneAtMaxLengthReachesCanonicalizer(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)
	h.expectActiveMerchant()
	h.expectNoExistingSession()

	input := h.partnerRequest()
	input.CustomerPhone = strings.Repeat("0", model.MaxCustomerPhoneLength)

	_, err := h.uc.CreatePartner(h.ctx, input)
	requireCerr(t, err, cerr.CodeInvalidArgument, "error.customer_phone_invalid")
	require.Empty(t, h.identityCalls)
	require.Empty(t, h.notificationCalls)
}

func TestCreatePartner_DisabledJurisdictionFailsBeforePhoneCanonicalize(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)
	h.merchantTx.EXPECT().
		GetByMerchantID(h.ctx, h.merchantID).
		Return(model.MerchantProfile{
			ID:               h.merchantPID,
			MerchantID:       h.merchantID,
			Status:           model.ActiveMerchantStatus,
			Jurisdiction:     model.Jurisdiction("TR"),
		}, nil).Once()
	h.expectNoExistingSession()

	_, err := h.uc.CreatePartner(h.ctx, h.partnerRequest())
	require.True(t, cerr.IsCode(err, cerr.CodeInternalError))
	require.False(t, cerr.IsCode(err, cerr.CodeInvalidArgument))
	require.Empty(t, h.identityCalls)
	require.Empty(t, h.notificationCalls)
}


func TestCreatePartner_IdempotentReplayAfterOriginationDisabled(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	// Replay must succeed without consulting origination admission.
	h.uc.jurisdictions = mock.NewOriginationJurisdictions(t)
	h.expectPool(2, 1)
	h.expectActiveMerchant()

	existing := h.session(func(s *model.CheckoutSession) { s.IdempotencyKey = testAttemptKey })
	h.sessionTx.EXPECT().
		LockByMerchantIdempotencyKey(h.ctx, h.merchantPID, testAttemptKey).
		Return(existing, nil).Once()
	h.expectPreparedExternalOfferReload(existing)

	output, err := h.uc.CreatePartner(h.ctx, h.partnerRequest())
	require.NoError(t, err)
	require.Equal(t, CreateResultAlreadyExists, output.Result)
	require.Equal(t, existing.ID, output.Session.ID)
	require.Empty(t, h.identityCalls)
	require.Empty(t, h.notificationCalls)
}

func TestCreatePartner_NewAttemptBlockedWhenOriginationDisabled(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	blocked := mock.NewOriginationJurisdictions(t)
	blocked.EXPECT().
		RequireOriginationEnabled(model.IRJurisdiction).
		Return(domaincapability.ErrOriginationNotEnabled).
		Once()
	h.uc.jurisdictions = blocked
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

	_, err := h.uc.CreatePartner(h.ctx, h.partnerRequest())
	require.True(t, cerr.IsCode(err, cerr.CodeInternalError))
	require.Empty(t, h.identityCalls)
	require.Empty(t, h.notificationCalls)
}

func TestCreatePartner_ConflictRecoveryDoesNotRequireOriginationAdmission(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	// Create race uses origination-enabled IR; recovery must still resolve the
	// winner even if origination is later blocked before the Conn recovery path.
	// Conn calls: create Tx, conflict recovery Conn, then two Conns for offer reload.
	h.expectPool(3, 1)
	h.expectActiveMerchant()
	h.expectKeyNotFound()

	previous := h.session(
		withStatus(model.ExpiredCheckoutSessionStatus),
		func(s *model.CheckoutSession) { s.IdempotencyKey = "attempt-0" },
	)
	h.sessionTx.EXPECT().
		LockByMerchantOrderID(h.ctx, h.merchantPID, testMerchantOrderID).
		Return(previous, nil).Once()

	branchID := model.MerchantBranchProfileID{}
	h.branchTx.EXPECT().
		LockByOnlineStoreID(h.ctx, h.storeID).
		Return(model.MerchantBranchProfile{ID: branchID, MerchantID: h.merchantID, OfferPreparationMode: model.OfferPreparationOnCreate}, nil).Once()
	h.sessionTx.EXPECT().
		GetReturnDestination(h.ctx, h.merchantID, testReturnDestination).
		Return(model.MerchantReturnDestination{
			MerchantID:      h.merchantID,
			DestinationName: testReturnDestination,
			ReturnURL:       "https://shop.example/return",
			Enabled:         true,
		}, nil).Once()
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
		RunAndReturn(func(
			_ context.Context,
			_ model.MerchantProfileID,
			_ model.MerchantBranchProfileID,
			_ model.OnlineStoreID,
			_ string,
			_ string,
			_ model.CheckoutSessionToken,
			_ model.CheckoutSessionOrigin,
			_ string,
			_ *string,
			_ *string,
			_ model.OfferPreparationMode,
			_ *model.CustomerID,
			_ string,
			_ string,
			_ string,
			_ uint64,
			_ model.Currency,
			_ model.CheckoutSessionStatus,
			_ time.Time,
			_ time.Time,
			_ time.Time,
		) (model.CheckoutSession, error) {
			// Simulate post-create config change before conflict recovery runs.
			// Recovery must not consult origination admission.
			h.uc.jurisdictions = mock.NewOriginationJurisdictions(t)

			return model.CheckoutSession{}, cerr.Conflict(errors.New("unique race"))
		}).
		Once()

	existing := h.session(func(s *model.CheckoutSession) { s.IdempotencyKey = testAttemptKey })
	h.merchantCQ.EXPECT().
		GetByMerchantID(h.ctx, h.merchantID).
		Return(model.MerchantProfile{
			ID:               h.merchantPID,
			MerchantID:       h.merchantID,
			Status:           model.ActiveMerchantStatus,
			Jurisdiction:     model.IRJurisdiction,
		}, nil).Once()
	h.sessionCQ.EXPECT().
		GetByMerchantIdempotencyKey(h.ctx, h.merchantPID, testAttemptKey).
		Return(existing, nil).Once()
	h.expectPreparedExternalOfferReload(existing)

	output, err := h.uc.CreatePartner(h.ctx, h.partnerRequest())
	require.NoError(t, err)
	require.Equal(t, CreateResultAlreadyExists, output.Result)
	require.Equal(t, existing.ID, output.Session.ID)
}
