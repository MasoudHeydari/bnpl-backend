package checkoutsessionuc

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/adapter/canonicalizer/phonenumber/iran"
	adapterclock "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	"github.com/parstis/bnpl-backend/pkg/core/canonicalizer/phonenumber"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/checkouturl"
	coreclock "github.com/parstis/bnpl-backend/pkg/core/clock"
	"github.com/parstis/bnpl-backend/pkg/core/domaincapability"
	"github.com/parstis/bnpl-backend/pkg/core/event/checkoutlifecycle"
	extcustomer "github.com/parstis/bnpl-backend/pkg/core/external/customer"
	extnotification "github.com/parstis/bnpl-backend/pkg/core/external/notification"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	checkoutcatalog "github.com/parstis/bnpl-backend/pkg/core/policy/checkout/catalog"
	checkoutorigin "github.com/parstis/bnpl-backend/pkg/core/policy/checkoutorigin"
	checkoutotppolicy "github.com/parstis/bnpl-backend/pkg/core/policy/checkoutotp"
	returndestination "github.com/parstis/bnpl-backend/pkg/core/policy/returndestination"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/contractuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/decisionuc"
)

const testMerchantEventsDestination = "bnpl.merchant.checkout.events.v1"

// settableClock is a test Clock whose Now can advance between phases without
// sleeping. Used to model lock-wait past CheckoutSession.expires_at.
type settableClock struct {
	adapterclock.Fixed
}

func newSettableClock(now time.Time) *settableClock {
	return &settableClock{Fixed: adapterclock.NewFixed(now, time.UTC)}
}

func (c *settableClock) set(now time.Time) {
	c.Time = now
}

var _ coreclock.Clock = (*settableClock)(nil)

func mustHarnessCheckoutURLs(t *testing.T) checkouturl.Builder {
	t.Helper()

	origin, err := checkoutorigin.Development([]string{"bnpl.example"}).Parse("https://bnpl.example")
	require.NoError(t, err)

	return checkouturl.New(origin)
}

func mustHarnessPublicToken(t *testing.T) model.CheckoutSessionToken {
	t.Helper()

	token, err := model.GenerateCheckoutSessionToken(strings.NewReader(
		"0123456789abcdef0123456789abcdef",
	))
	require.NoError(t, err)

	return token
}

var errFinancialCoreDown = errors.New("financial core down")

// fakeContractCreator records CreateContract calls so tests can prove a Contract
// was or was not created.
type fakeContractCreator struct {
	calls     []contractuc.CreateContractInput
	output    contractuc.CreateContractOutput
	submitted *contractuc.CreateContractOutput
	submitErr error
	err       error
}

func (f *fakeContractCreator) CreateContract(
	_ context.Context,
	input contractuc.CreateContractInput,
) (contractuc.CreateContractOutput, error) {
	f.calls = append(f.calls, input)

	return f.output, f.err
}

// The prepare fake records the logical creation, and the submit fake replays
// the same response while preserving strict expectations on creation count.
func (f *fakeContractCreator) PrepareCheckoutContract(ctx context.Context, input contractuc.CreateContractInput) (contractuc.CreateContractOutput, error) {
	return f.CreateContract(ctx, input)
}

func (f *fakeContractCreator) SubmitPreparedCheckoutFulfillment(_ context.Context, prepared contractuc.CreateContractOutput) (contractuc.CreateContractOutput, error) {
	if f.submitErr != nil {
		return contractuc.CreateContractOutput{}, f.submitErr
	}
	if f.submitted != nil {
		return *f.submitted, nil
	}
	return prepared, nil
}

// fakeDecisionCreator records decision Create / RevalidateCheckoutOffer calls.
type fakeDecisionCreator struct {
	calls            []decisionuc.CreateInput
	output           decisionuc.CreateOutput
	err              error
	revalidateCalls  []decisionuc.RevalidateCheckoutOfferInput
	revalidateOutput decisionuc.RevalidateCheckoutOfferOutput
	revalidateErr    error
}

func (f *fakeDecisionCreator) Create(
	_ context.Context,
	input decisionuc.CreateInput,
) (decisionuc.CreateOutput, error) {
	f.calls = append(f.calls, input)

	return f.output, f.err
}

func (f *fakeDecisionCreator) RevalidateCheckoutOffer(
	_ context.Context,
	input decisionuc.RevalidateCheckoutOfferInput,
) (decisionuc.RevalidateCheckoutOfferOutput, error) {
	f.revalidateCalls = append(f.revalidateCalls, input)

	return f.revalidateOutput, f.revalidateErr
}

// harness wires a UseCase to strict mockery mocks. Every repository call that
// is not explicitly expected fails the test, which is how the tests prove that
// forbidden writes (MarkCanceled, MarkConfirming, UpdateStatus, ...) never run.
type harness struct {
	t   *testing.T
	ctx context.Context
	now time.Time

	pool *mock.Pool
	conn *mock.Conn
	tx   *mock.Tx

	sessionTx    *mock.CheckoutSessionTxQueryer
	merchantTx   *mock.MerchantProfileTxQueryer
	merchantCQ   *mock.MerchantProfileConnQueryer
	branchTx     *mock.MerchantBranchProfileTxQueryer
	// branchOfferMode is returned by LockByOnlineStoreID during Create. Default
	// on_create matches existing Partner Create tests that expect an Offer.
	branchOfferMode model.OfferPreparationMode
	customerTx      *mock.CustomerProfileTxQueryer
	customerCQ   *mock.CustomerProfileConnQueryer
	sessionCQ    *mock.CheckoutSessionConnQueryer
	decisionCQ   *mock.DecisionConnQueryer
	decisionTx   *mock.DecisionTxQueryer
	allocationCQ *mock.CheckoutDecisionAllocationConnQueryer
	installCQ    *mock.InstallmentConnQueryer
	chargeCQ     *mock.InstallmentChargeConnQueryer
	outboxTx     *mock.OutboxTxQueryer
	contractTx   *mock.ContractTxQueryer
	contractCQ   *mock.ContractConnQueryer
	attemptTx    *mock.PaymentAttemptTxQueryer
	attemptCQ    *mock.PaymentAttemptConnQueryer
	installTx    *mock.InstallmentTxQueryer
	chargeTx     *mock.InstallmentChargeTxQueryer
	contracts    *fakeContractCreator
	decisions    *fakeDecisionCreator

	identity             *mock.CustomerResolver
	identityErr          error
	identityOutput       extcustomer.ResolveByMobileOutput
	identityCalls        []extcustomer.ResolveByMobileInput
	notification         *mock.NotificationSubmitter
	notificationErr      error
	notificationCalls    []extnotification.SubmitInput
	phoneCanonicalizer   *mock.PhoneCanonicalizer
	phoneCanonicalizeErr error
	jurisdictions        domaincapability.OriginationJurisdictions
	customerPhoneBox     *mock.SecretBox
	webhookSecretBox     *mock.SecretBox

	uc          *UseCase
	sessionID   model.CheckoutSessionID
	publicToken model.CheckoutSessionToken
	decisionID  model.DecisionID
	contractID  model.ContractID
	attemptID   model.PaymentAttemptID
	customerID  model.CustomerID
	profileID   model.ExternalCustomerID
	merchantPID model.MerchantProfileID
	merchantID  model.MerchantID
	storeID     model.OnlineStoreID
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	now := time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC)

	h := &harness{
		t:               t,
		ctx:             context.Background(),
		now:             now,
		pool:            mock.NewPool(t),
		conn:            mock.NewConn(t),
		tx:              mock.NewTx(t),
		sessionTx:       mock.NewCheckoutSessionTxQueryer(t),
		merchantTx:      mock.NewMerchantProfileTxQueryer(t),
		merchantCQ:      mock.NewMerchantProfileConnQueryer(t),
		branchTx:        mock.NewMerchantBranchProfileTxQueryer(t),
		branchOfferMode: model.OfferPreparationOnCreate,
		customerTx:   mock.NewCustomerProfileTxQueryer(t),
		customerCQ:   mock.NewCustomerProfileConnQueryer(t),
		sessionCQ:    mock.NewCheckoutSessionConnQueryer(t),
		decisionCQ:   mock.NewDecisionConnQueryer(t),
		decisionTx:   mock.NewDecisionTxQueryer(t),
		allocationCQ: mock.NewCheckoutDecisionAllocationConnQueryer(t),
		installCQ:    mock.NewInstallmentConnQueryer(t),
		chargeCQ:     mock.NewInstallmentChargeConnQueryer(t),
		outboxTx:     mock.NewOutboxTxQueryer(t),
		contractTx:   mock.NewContractTxQueryer(t),
		contractCQ:   mock.NewContractConnQueryer(t),
		attemptTx:    mock.NewPaymentAttemptTxQueryer(t),
		attemptCQ:    mock.NewPaymentAttemptConnQueryer(t),
		installTx:    mock.NewInstallmentTxQueryer(t),
		chargeTx:     mock.NewInstallmentChargeTxQueryer(t),
		contracts:    &fakeContractCreator{},
		decisions: &fakeDecisionCreator{
			revalidateOutput: decisionuc.RevalidateCheckoutOfferOutput{StillHonorable: true},
		},
		identity:           mock.NewCustomerResolver(t),
		notification:       mock.NewNotificationSubmitter(t),
		phoneCanonicalizer: mock.NewPhoneCanonicalizer(t),
		customerPhoneBox:   mock.NewSecretBox(t),
		webhookSecretBox:   mock.NewSecretBox(t),
		sessionID:          model.CheckoutSessionID(uuid.New()),
		publicToken:        mustHarnessPublicToken(t),
		decisionID:         model.DecisionID(uuid.New()),
		contractID:         model.ContractID(uuid.New()),
		attemptID:          model.PaymentAttemptID(uuid.New()),
		customerID:         model.CustomerID(uuid.New()),
		profileID:          model.ExternalCustomerID(uuid.New()),
		merchantPID:        model.MerchantProfileID(uuid.New()),
		merchantID:         model.MerchantID(uuid.New()),
		storeID:            model.OnlineStoreID(uuid.New()),
	}

	jurisdictions, err := domaincapability.NewJurisdictions([]model.Jurisdiction{model.IRJurisdiction})
	if err != nil {
		t.Fatalf("new jurisdiction capabilities: %v", err)
	}

	h.jurisdictions = jurisdictions

	h.installDefaultCoreCollaborators()

	sessionRepo := mock.NewCheckoutSession(t)
	merchantRepo := mock.NewMerchantProfile(t)
	branchRepo := mock.NewMerchantBranchProfile(t)
	customerRepo := mock.NewCustomerProfile(t)
	decisionRepo := mock.NewDecision(t)
	outboxRepo := mock.NewOutbox(t)
	contractRepo := mock.NewContract(t)
	attemptRepo := mock.NewPaymentAttempt(t)
	installRepo := mock.NewInstallment(t)
	chargeRepo := mock.NewInstallmentCharge(t)

	sessionRepo.EXPECT().Tx(h.tx).Return(h.sessionTx).Maybe()
	sessionRepo.EXPECT().Conn(h.conn).Return(h.sessionCQ).Maybe()
	merchantRepo.EXPECT().Tx(h.tx).Return(h.merchantTx).Maybe()
	merchantRepo.EXPECT().Conn(h.conn).Return(h.merchantCQ).Maybe()
	branchRepo.EXPECT().Tx(h.tx).Return(h.branchTx).Maybe()
	customerRepo.EXPECT().Tx(h.tx).Return(h.customerTx).Maybe()
	customerRepo.EXPECT().Conn(h.conn).Return(h.customerCQ).Maybe()
	decisionRepo.EXPECT().Conn(h.conn).Return(h.decisionCQ).Maybe()
	decisionRepo.EXPECT().Tx(h.tx).Return(h.decisionTx).Maybe()
	outboxRepo.EXPECT().Tx(h.tx).Return(h.outboxTx).Maybe()
	contractRepo.EXPECT().Tx(h.tx).Return(h.contractTx).Maybe()
	contractRepo.EXPECT().Conn(h.conn).Return(h.contractCQ).Maybe()
	attemptRepo.EXPECT().Tx(h.tx).Return(h.attemptTx).Maybe()
	attemptRepo.EXPECT().Conn(h.conn).Return(h.attemptCQ).Maybe()
	installRepo.EXPECT().Conn(h.conn).Return(h.installCQ).Maybe()
	installRepo.EXPECT().Tx(h.tx).Return(h.installTx).Maybe()
	chargeRepo.EXPECT().Conn(h.conn).Return(h.chargeCQ).Maybe()
	chargeRepo.EXPECT().Tx(h.tx).Return(h.chargeTx).Maybe()

	lifecycle, err := NewLifecycleOutbox(
		sessionRepo,
		merchantRepo,
		outboxRepo,
		mock.NewIdentifierGenerator(t),
		testMerchantEventsDestination,
	)
	if err != nil {
		t.Fatalf("new lifecycle outbox: %v", err)
	}

	allocationRepo := mock.NewCheckoutDecisionAllocation(t)
	allocationRepo.EXPECT().Conn(h.conn).Return(h.allocationCQ).Maybe()
	allocationRepo.EXPECT().Tx(h.tx).Return(mock.NewCheckoutDecisionAllocationTxQueryer(t)).Maybe()

	checkoutPolicies, err := checkoutcatalog.NewRegistry()
	if err != nil {
		t.Fatalf("new checkout policy registry: %v", err)
	}

	h.uc = &UseCase{
		pool:                       h.pool,
		checkoutSession:            sessionRepo,
		merchantProfile:            merchantRepo,
		merchantBranchProfile:      branchRepo,
		customerProfile:            customerRepo,
		contract:                   contractRepo,
		paymentAttempt:             attemptRepo,
		installment:                installRepo,
		installmentCharge:          chargeRepo,
		decisionRepo:               decisionRepo,
		checkoutDecisionAllocation: allocationRepo,
		checkoutPolicies:           checkoutPolicies,
		decision:                   h.decisions,
		contractUC:                 h.contracts,
		lifecycle:                  lifecycle,
		random:                     rand.Reader,
		webhookSecretBox:           h.webhookSecretBox,
		customerPhoneBox:           h.customerPhoneBox,
		returnURLPolicy:            returndestination.Development(),
		clock:                      adapterclock.NewFixed(now, time.UTC),
		checkoutURLs:               mustHarnessCheckoutURLs(h.t),
		ttl:                        30 * time.Minute,
		expiryBatchSize:            25,
		otpHasher:                  newTestOTPHasher(t),
		customerResolver:           h.identity,
		notificationSubmitter:      h.notification,
		jurisdictions:              h.jurisdictions,
		phoneCanonicalizer:         h.phoneCanonicalizer,
		otpPolicy:                  checkoutotppolicy.V1(),
		otpTemplateID:              testOTPTemplateID,
	}

	return h
}

// installDefaultCoreCollaborators wires mockery mocks for Core ports with
// default behaviors that match the previous hand-rolled stubs.
func (h *harness) installDefaultCoreCollaborators() {
	h.t.Helper()

	iranCanon := iran.New()

	h.phoneCanonicalizer.EXPECT().
		Canonicalize(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(jurisdiction model.Jurisdiction, raw string) (string, error) {
			if h.phoneCanonicalizeErr != nil {
				return "", h.phoneCanonicalizeErr
			}

			if jurisdiction != model.IRJurisdiction {
				return "", phonenumber.ErrUnsupportedJurisdiction
			}

			return iranCanon.Canonicalize(raw)
		}).
		Maybe()

	h.identity.EXPECT().
		ResolveByMobile(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(
			_ context.Context,
			input extcustomer.ResolveByMobileInput,
		) (extcustomer.ResolveByMobileOutput, error) {
			h.identityCalls = append(h.identityCalls, input)
			if h.identityErr != nil {
				return extcustomer.ResolveByMobileOutput{}, h.identityErr
			}

			if !h.identityOutput.ExternalCustomerID.IsZero() || h.identityOutput.Status != "" {
				return h.identityOutput, nil
			}

			return extcustomer.ResolveByMobileOutput{
				ExternalCustomerID: h.profileID,
				Status:             extcustomer.ActiveStatus,
			}, nil
		}).
		Maybe()

	h.notification.EXPECT().
		Submit(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(
			_ context.Context,
			input extnotification.SubmitInput,
		) (extnotification.SubmitOutput, error) {
			h.notificationCalls = append(h.notificationCalls, input)
			if h.notificationErr != nil {
				return extnotification.SubmitOutput{}, h.notificationErr
			}

			return extnotification.SubmitOutput{RequestID: "req-1"}, nil
		}).
		Maybe()

	h.customerPhoneBox.EXPECT().
		Encrypt(testifymock.Anything).
		RunAndReturn(func(plaintext string) (string, string, error) {
			return "sb1." + plaintext, "test", nil
		}).
		Maybe()
	h.customerPhoneBox.EXPECT().
		Decrypt(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(ciphertext string, _ string) (string, error) {
			return strings.TrimPrefix(ciphertext, "sb1."), nil
		}).
		Maybe()

	h.webhookSecretBox.EXPECT().
		Encrypt(testifymock.Anything).
		RunAndReturn(func(plaintext string) (string, string, error) {
			return "sb1." + plaintext, "test", nil
		}).
		Maybe()
	h.webhookSecretBox.EXPECT().
		Decrypt(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(ciphertext string, _ string) (string, error) {
			return strings.TrimPrefix(ciphertext, "sb1."), nil
		}).
		Maybe()
}

// expectPool registers connCalls Pool.Conn invocations that never open a
// transaction plus txCalls Pool.Conn invocations that immediately open one.

// expectStartExternalPrepareLoad expects the deferred-prepare mode probe at START.
func (h *harness) expectStartExternalPrepareLoad() {
	h.t.Helper()
	h.sessionCQ.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(h.session(), nil).Once()
}

func (h *harness) expectPool(connCalls int, txCalls int) {
	h.t.Helper()

	total := connCalls + txCalls
	if total > 0 {
		h.pool.EXPECT().Conn(h.ctx, testifymock.Anything).RunAndReturn(
			func(ctx context.Context, handler repo.ConnHandler) error {
				return handler(ctx, h.conn)
			},
		).Times(total)
	}

	if txCalls > 0 {
		h.conn.EXPECT().Tx(h.ctx, testifymock.Anything).RunAndReturn(
			func(ctx context.Context, handler repo.TxHandler) error {
				return handler(ctx, h.tx)
			},
		).Times(txCalls)
	}
}

// session returns an open, customer-bound, decision-linked checkout session at
// evaluation generation 1 that expires in the future. mods adjust the row.
func (h *harness) session(mods ...func(*model.CheckoutSession)) model.CheckoutSession {
	customerID := h.customerID
	decisionID := h.decisionID

	phoneCiphertext := "sb1.+989121234567"
	phoneKeyID := "test"
	session := model.CheckoutSession{
		ID:                      h.sessionID,
		PublicToken:             h.publicToken,
		Origin:                  model.ExternalPartnerCheckoutSessionOrigin,
		MerchantProfileID:       h.merchantPID,
		OnlineStoreID:           h.storeID,
		MerchantOrderID:         "order-1",
		IdempotencyKey:          "order-1",
		CustomerReference:       "cust-1",
		CustomerPhoneCiphertext: &phoneCiphertext,
		CustomerPhoneKeyID:      &phoneKeyID,
		OfferPreparationMode:    model.OfferPreparationOnCreate,
		CustomerID:              &customerID,
		ReturnDestination:       "storefront",
		ReturnURL:               "https://shop.example/return",
		MerchantState:           "cart-42",
		RequestedAmountMinor:    150000000,
		Currency:                model.IRRCurrency,
		Status:                  model.OpenCheckoutSessionStatus,
		DecisionID:              &decisionID,
		EvaluationGeneration:    1,
		ExpiresAt:               h.now.Add(10 * time.Minute),
		CreatedAt:               h.now.Add(-20 * time.Minute),
		UpdatedAt:               h.now.Add(-time.Minute),
	}

	for _, mod := range mods {
		mod(&session)
	}

	return session
}

func withStatus(status model.CheckoutSessionStatus) func(*model.CheckoutSession) {
	return func(s *model.CheckoutSession) { s.Status = status }
}

func withContract(id model.ContractID) func(*model.CheckoutSession) {
	return func(s *model.CheckoutSession) { s.ContractID = &id }
}

func withoutDecision() func(*model.CheckoutSession) {
	return func(s *model.CheckoutSession) { s.DecisionID = nil }
}

func withGeneration(generation int) func(*model.CheckoutSession) {
	return func(s *model.CheckoutSession) { s.EvaluationGeneration = generation }
}

func withExpiresAt(at time.Time) func(*model.CheckoutSession) {
	return func(s *model.CheckoutSession) { s.ExpiresAt = at }
}

func withCustomer(id *model.CustomerID) func(*model.CheckoutSession) {
	return func(s *model.CheckoutSession) { s.CustomerID = id }
}

func (h *harness) customerProfile() model.CustomerProfile {
	return model.CustomerProfile{ID: h.customerID, ExternalCustomerID: h.profileID}
}

// expectCustomerLock expects the customer profile row lock Confirm and cancel take.
func (h *harness) expectCustomerLock() {
	h.customerTx.EXPECT().
		LockByExternalCustomerID(h.ctx, h.profileID).
		Return(h.customerProfile(), nil).Once()
}

// expectLifecycleEvent expects exactly one merchant lifecycle outbox row of
// eventType for session. The merchant has no webhook destination.
func (h *harness) expectLifecycleEvent(session model.CheckoutSession, eventType string) {
	h.t.Helper()

	h.merchantTx.EXPECT().
		GetByID(h.ctx, session.MerchantProfileID).
		Return(model.MerchantProfile{
			ID:         session.MerchantProfileID,
			MerchantID: h.merchantID,
			Status:     model.ActiveMerchantStatus,
		}, nil).Once()
	h.sessionTx.EXPECT().
		GetWebhookDestination(h.ctx, h.merchantID).
		Return(model.MerchantWebhookDestination{}, cerr.ResourceNotFound(errors.New("no webhook"))).Once()
	h.outboxTx.EXPECT().
		Create(h.ctx, testifymock.MatchedBy(func(message repo.CreateOutboxMessage) bool {
			if message.Destination != testMerchantEventsDestination || message.EventType != eventType {
				return false
			}

			var event checkoutlifecycle.Event
			if err := json.Unmarshal(message.Payload, &event); err != nil {
				return false
			}

			return event.EventID == session.PublicToken.String()+":"+eventType+":"+string(session.Status) &&
				event.Data.CheckoutSessionID == session.PublicToken.String() &&
				event.Data.Status == string(session.Status)
		})).
		Return(model.OutboxMessage{}, nil).Once()
}

// requireCerr asserts err is a public application error with the given code and
// message key.
func requireCerr(t *testing.T, err error, code cerr.Code, messageKey cerr.MessageKey) {
	t.Helper()

	require.Error(t, err)

	var appErr *cerr.Error
	require.True(t, errors.As(err, &appErr), "expected *cerr.Error, got %T: %v", err, err)
	require.Equal(t, code, appErr.Code)
	require.Equal(t, messageKey, appErr.MessageKey)
}

func (h *harness) confirmInput(generation int) ConfirmInput {
	return ConfirmInput{
		CheckoutSessionID:    h.sessionID,
		ExternalCustomerID:    h.profileID,
		TenantID:             model.TenantID(uuid.New()),
		ContextID:            model.ContextID(uuid.New()),
		EvaluationGeneration: generation,
	}
}

// createdContract is the CreateContract result Confirm links to the session.
func (h *harness) createdContract(paymentRequired bool) contractuc.CreateContractOutput {
	status := model.ActiveContractStatus
	if paymentRequired {
		status = model.PaymentPendingContractStatus
	}

	return contractuc.CreateContractOutput{
		Contract: model.Contract{
			ID:     h.contractID,
			Status: status,
		},
		PaymentRequired: paymentRequired,
		PaymentAttempt: model.PaymentAttempt{
			ID:         h.attemptID,
			ContractID: h.contractID,
			Purpose:    model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
		},
	}
}

func (h *harness) expectCheckoutPaymentAttemptLoad(paymentRequired bool) {
	h.t.Helper()

	if !paymentRequired {
		return
	}

	h.attemptTx.EXPECT().GetByContractIDAndPurposeWithItems(
		h.ctx,
		h.contractID,
		model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
	).Return(h.createdContract(true).PaymentAttempt, nil).Once()
}

func (h *harness) expectOTPRateLimits() {
	h.t.Helper()
	h.sessionTx.EXPECT().
		ConsumeOTPRateLimit(h.ctx, model.CheckoutOTPRateLimitScopePhone, testifymock.AnythingOfType("string"), 10*time.Minute, 5, h.now).
		Return(nil).Once()
	h.sessionTx.EXPECT().
		ConsumeOTPRateLimit(h.ctx, model.CheckoutOTPRateLimitScopeMerchant, h.merchantPID.String(), 10*time.Minute, 30_000, h.now).
		Return(nil).Once()
}

func (h *harness) startExternalInput(generation int) StartExternalInput {
	return StartExternalInput{
		Token:                h.publicToken,
		EvaluationGeneration: generation,
	}
}

func (h *harness) approvedDecisionExpiresAt() time.Time {
	return h.now.Add(time.Hour)
}

func (h *harness) setApprovedDecisionCreateOutput() {
	expires := h.approvedDecisionExpiresAt()
	h.decisions.output = decisionuc.CreateOutput{
		Decision: model.Decision{
			ID:            h.decisionID,
			Outcome:       model.ApprovedDecisionOutcome,
			PolicyID:      "checkout_contract_approval",
			PolicyVersion: "v1",
			ExpiresAt:     &expires,
		},
		RequestedAmountMinor:        testRequestedAmount,
		ApprovedFinancedAmountMinor: testRequestedAmount,
		UpfrontAmountMinor:          0,
		AvailableCreditMinor:        500_000_000,
		Currency:                    model.IRRCurrency,
		ExpiresAt:                   &expires,
		InstallmentOffer: &decisionuc.InstallmentOffer{
			InstallmentCount:        4,
			PayableNowAmountMinor:   testRequestedAmount / 4,
			TotalPayableAmountMinor: testRequestedAmount,
			Currency:                model.IRRCurrency,
			ExpiresAt:               expires,
		},
	}
}

func (h *harness) persistedApprovedDecision() model.Decision {
	expires := h.approvedDecisionExpiresAt()

	return model.Decision{
		ID:            h.decisionID,
		Outcome:       model.ApprovedDecisionOutcome,
		PolicyID:      "checkout_contract_approval",
		PolicyVersion: "v1",
		ExpiresAt:     &expires,
	}
}

func (h *harness) persistedApprovedAllocation() model.CheckoutDecisionAllocation {
	expires := h.approvedDecisionExpiresAt()

	return model.CheckoutDecisionAllocation{
		DecisionID:                  h.decisionID,
		CustomerID:                  h.customerID,
		MerchantProfileID:           h.merchantPID,
		RequestedAmountMinor:        testRequestedAmount,
		ApprovedFinancedAmountMinor: testRequestedAmount,
		UpfrontAmountMinor:          0,
		AvailableCreditMinor:        500_000_000,
		Currency:                    model.IRRCurrency,
		InstallmentCount:            4,
		ExpiresAt:                   expires,
	}
}

func (h *harness) persistedApprovedInstallments() []model.Installment {
	per := testRequestedAmount / 4
	installments := make([]model.Installment, 0, 4)

	for order := 1; order <= 4; order++ {
		installments = append(installments, model.Installment{
			ID:                   model.InstallmentID(uuid.New()),
			DecisionID:           h.decisionID,
			CustomerID:           h.customerID,
			MerchantProfileID:    h.merchantPID,
			InstallmentOrder:     order,
			Currency:             model.IRRCurrency,
			PrincipalAmountMinor: per,
			TotalAmountMinor:     per,
			DueAt:                h.now.Add(time.Duration(order) * 24 * time.Hour),
			Status:               model.OfferedInstallmentStatus,
		})
	}

	return installments
}

// expectProjectApprovedOffer wires Conn reads for projectPersistedOffer.
func (h *harness) expectProjectApprovedOffer(session model.CheckoutSession) {
	h.t.Helper()

	require.NotNil(h.t, session.DecisionID)

	installments := h.persistedApprovedInstallments()
	h.decisionCQ.EXPECT().
		GetByID(h.ctx, *session.DecisionID).
		Return(h.persistedApprovedDecision(), nil).Once()
	h.allocationCQ.EXPECT().
		GetByDecisionID(h.ctx, *session.DecisionID).
		Return(h.persistedApprovedAllocation(), nil).Once()
	h.installCQ.EXPECT().
		ListByDecisionID(h.ctx, *session.DecisionID).
		Return(installments, nil).Once()
	h.chargeCQ.EXPECT().
		ListByInstallmentIDs(h.ctx, testifymock.Anything).
		Return([]model.InstallmentCharge{}, nil).Once()
}

func (h *harness) expectProjectDeniedOffer(session model.CheckoutSession) {
	h.t.Helper()

	require.NotNil(h.t, session.DecisionID)

	h.decisionCQ.EXPECT().
		GetByID(h.ctx, *session.DecisionID).
		Return(model.Decision{
			ID:      *session.DecisionID,
			Outcome: model.DeniedDecisionOutcome,
		}, nil).Once()
}

// expectPreparedExternalOfferReload expects loadPreparedExternalOffer on replay,
// including the full persisted Offer projection (Decision + allocation +
// installments + charges).
func (h *harness) expectPreparedExternalOfferReload(session model.CheckoutSession) {
	h.t.Helper()

	require.NotNil(h.t, session.DecisionID)

	// loadPreparedExternalOffer loads the Decision once before projecting.
	h.decisionCQ.EXPECT().
		GetByID(h.ctx, *session.DecisionID).
		Return(h.persistedApprovedDecision(), nil).Once()
	h.expectProjectApprovedOffer(session)
}

// expectExternalOfferPreparation expects Identity bind + Decision evaluate after create.
func (h *harness) expectExternalOfferPreparation(unbound, bound, linked model.CheckoutSession) {
	h.t.Helper()

	h.setApprovedDecisionCreateOutput()
	h.expectExternalBind(unbound, bound)
	h.sessionTx.EXPECT().LockByID(h.ctx, unbound.ID).Return(bound, nil).Once()
	h.sessionTx.EXPECT().
		LinkDecision(h.ctx, unbound.ID, h.decisionID, 1, h.now).
		Return(linked, nil).Once()
}
