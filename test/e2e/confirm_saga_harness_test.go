//go:build e2e

package e2e_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/cmd/bnpl/bootstrap"
	"github.com/parstis/bnpl-backend/pkg/adapter/config"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	extcontractcancellationnoop "github.com/parstis/bnpl-backend/pkg/adapter/external/contractcancellation/noop"
	extnotificationnoop "github.com/parstis/bnpl-backend/pkg/adapter/external/notification/noop"
	adapterclock "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	metricsnoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/metrics/noop"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	extcustomer "github.com/parstis/bnpl-backend/pkg/core/external/customer"
	extnotification "github.com/parstis/bnpl-backend/pkg/core/external/notification"
	"github.com/parstis/bnpl-backend/pkg/core/fingerprint"
	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	checkoutcatalog "github.com/parstis/bnpl-backend/pkg/core/policy/checkout/catalog"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/saga"
	"github.com/parstis/bnpl-backend/pkg/core/trace"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/checkoutsessionuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/contractuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/decisionuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/financialcorepaymentresultuc"
	otpusecase "github.com/parstis/bnpl-backend/pkg/core/usecase/otp"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/sagaexecutoruc"
)

// fixedCustomerResolver returns a configured ExternalCustomerID for every mobile.
type fixedCustomerResolver struct {
	externalCustomerID model.ExternalCustomerID
}

func (f fixedCustomerResolver) ResolveByMobile(
	_ context.Context,
	_ extcustomer.ResolveByMobileInput,
) (extcustomer.ResolveByMobileOutput, error) {
	return extcustomer.ResolveByMobileOutput{
		ExternalCustomerID: f.externalCustomerID,
		Status:             extcustomer.ActiveStatus,
	}, nil
}

// confirmSagaHarness wires real Postgres repositories and checkout confirmation
// saga execution with a controllable Financial Core client and optional barriers.
type confirmSagaHarness struct {
	t   *testing.T
	ctx context.Context

	pool              repo.Pool
	fc                *controllableCheckoutPaymentClient
	clock             *adapterclock.Fixed
	logger            corelog.Logger
	fingerprintHasher fingerprint.Hasher
	repos             bootstrap.Repos

	CheckoutSession            *checkoutsessionuc.UseCase
	OTP                        *otpusecase.UseCase
	PaymentResult              *financialcorepaymentresultuc.UseCase
	SagaExecutor               *sagaexecutoruc.UseCase
	confirmationSagaDefinition *checkoutsessionuc.ConfirmationSagaDefinition
	cancellationSagaDefinition saga.Definition
	definitions                saga.DefinitionResolver
}

type confirmSagaHarnessOptions struct {
	hooks                                    checkoutsessionuc.ConfirmationSagaHooks
	externalCustomerID                       model.ExternalCustomerID
	beforeTerminalCheckoutPaymentReplacement func(context.Context) error
	notificationSubmitter                    extnotification.Submitter
}

func newConfirmSagaHarness(t *testing.T, opts confirmSagaHarnessOptions) *confirmSagaHarness {
	t.Helper()

	if opts.externalCustomerID.IsZero() {
		opts.externalCustomerID = model.ExternalCustomerID(uuid.New())
	}

	root, err := repositoryRoot()
	require.NoError(t, err)

	cfg, err := config.Load(filepath.Join(root, "configs/runtime.yaml"))
	require.NoError(t, err)

	app := &bootstrap.App{Config: cfg, Repos: bootstrap.NewRepos()}
	infra, err := app.NewInfrastructures(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() { _ = infra.Close() })

	fc := newControllableCheckoutPaymentClient()
	now := time.Date(
		2026,
		10,
		18,
		9,
		0,
		0,
		0,
		time.UTC,
	)
	clk := adapterclock.NewFixed(now, time.UTC)
	fixedClock := &clk

	pool := &postgres.Pool{Pool: testDB}
	repos := app.Repos

	checkoutPolicies, err := checkoutcatalog.NewRegistry()
	require.NoError(t, err)

	jurisdictions, err := cfg.NewJurisdictionCapabilities()
	require.NoError(t, err)

	businessMetrics := metricsnoop.New()

	decisionUC, err := cfg.NewDecisionUseCase(
		pool,
		repos.Decision,
		repos.CustomerProfile,
		repos.CreditAccount,
		repos.MerchantProfile,
		repos.MerchantBranchProfile,
		repos.CheckoutDecisionAllocation,
		repos.PolicyAssignment,
		repos.Installment,
		repos.InstallmentCharge,
		repos.Outbox,
		fixedClock,
		infra.FingerprintHasher,
		checkoutPolicies,
		checkoutcatalog.DefaultRef(),
		infra.IdentifierGenerator,
		jurisdictions,
		businessMetrics,
		decisionuc.AllowAllNewCreditUsage{},
	)
	require.NoError(t, err)

	cancellationClient := extcontractcancellationnoop.New()
	cancellationSagaDefinition, err := cfg.NewContractCancellationSagaDefinition(
		pool,
		repos.Contract,
		repos.CreditAccount,
		cancellationClient,
		fixedClock,
	)
	require.NoError(t, err)

	cancellationExecutor, err := cfg.NewSagaExecutorUseCase(
		pool,
		repos.Saga,
		fixedClock,
		cancellationSagaDefinition,
	)
	require.NoError(t, err)

	contractUC, err := cfg.NewContractUseCase(
		pool,
		repos.CustomerProfile,
		repos.MerchantProfile,
		repos.CreditAccount,
		repos.Decision,
		repos.Contract,
		repos.CheckoutDecisionAllocation,
		checkoutPolicies,
		repos.Installment,
		repos.InstallmentCharge,
		repos.PaymentAttempt,
		fc,
		cancellationClient,
		repos.Outbox,
		fixedClock,
		infra.IdentifierGenerator,
		repos.Saga,
		cancellationExecutor,
		cancellationSagaDefinition,
		jurisdictions,
		infra.Logger,
		businessMetrics,
		contractuc.AllowAllNewCreditUsage{},
	)
	require.NoError(t, err)

	lifecycle, err := checkoutsessionuc.NewLifecycleOutbox(
		repos.CheckoutSession,
		repos.MerchantProfile,
		repos.Outbox,
		infra.IdentifierGenerator,
		cfg.Usecases.CheckoutSession.LifecycleEventsDestination,
	)
	require.NoError(t, err)

	sagaRetry := cfg.Usecases.Sagas.CheckoutConfirmation
	sagaRetry.ApplyDefaults()

	if opts.beforeTerminalCheckoutPaymentReplacement != nil {
		contractUC.BeforeTerminalCheckoutPaymentReplacement = opts.beforeTerminalCheckoutPaymentReplacement
	}

	confirmationSagaDefinition, err := checkoutsessionuc.NewConfirmationSagaDefinition(
		pool,
		repos.CheckoutSession,
		repos.CustomerProfile,
		repos.Contract,
		repos.PaymentAttempt,
		repos.Saga,
		contractUC,
		lifecycle,
		fixedClock,
		sagaRetry.RetryBaseDelay,
		sagaRetry.RetryMaxDelay,
		sagaRetry.MaxRetryAttempts,
		opts.hooks,
	)
	require.NoError(t, err)

	sagaExecutor, err := cfg.NewSagaExecutorUseCase(
		pool,
		repos.Saga,
		fixedClock,
		cancellationSagaDefinition,
		confirmationSagaDefinition,
	)
	require.NoError(t, err)

	webhookSecretBox, err := cfg.Usecases.CheckoutSession.NewWebhookSecretBox()
	require.NoError(t, err)

	customerPhoneBox, err := cfg.Usecases.CheckoutSession.NewCustomerPhoneBox()
	require.NoError(t, err)

	customerResolver := fixedCustomerResolver{externalCustomerID: opts.externalCustomerID}
	notificationSubmitter := opts.notificationSubmitter
	if notificationSubmitter == nil {
		notificationSubmitter = extnotificationnoop.New()
	}

	otpUC, err := cfg.NewOTPUseCase(
		pool,
		repos.OTP,
		notificationSubmitter,
		fixedClock,
	)
	require.NoError(t, err)

	checkoutUC, err := cfg.NewCheckoutSessionUseCase(
		pool,
		repos.CheckoutSession,
		repos.MerchantProfile,
		repos.MerchantBranchProfile,
		repos.CustomerProfile,
		repos.Contract,
		repos.PaymentAttempt,
		repos.Installment,
		repos.InstallmentCharge,
		repos.Decision,
		repos.CheckoutDecisionAllocation,
		checkoutPolicies,
		decisionUC,
		contractUC,
		repos.Outbox,
		infra.IdentifierGenerator,
		webhookSecretBox,
		customerPhoneBox,
		fixedClock,
		customerResolver,
		jurisdictions,
		otpUC,
		repos.Saga,
		sagaExecutor,
		confirmationSagaDefinition,
	)
	require.NoError(t, err)

	paymentResultUC, err := cfg.NewFinancialCorePaymentResultUseCase(
		pool,
		contractUC,
		checkoutUC,
		fc,
		repos.Inbox,
		repos.Outbox,
		repos.PaymentAttempt,
		repos.Contract,
		repos.Installment,
		repos.InstallmentCharge,
		repos.CreditAccount,
		infra.IdentifierGenerator,
		infra.FingerprintHasher,
		fixedClock,
		businessMetrics,
	)
	require.NoError(t, err)

	ctx := trace.WithTraceID(context.Background(), uuid.NewString())
	ctx = trace.WithCorrelationID(ctx, uuid.NewString())

	registry, err := saga.NewRegistry(cancellationSagaDefinition, confirmationSagaDefinition)
	require.NoError(t, err)

	return &confirmSagaHarness{
		t:                          t,
		ctx:                        ctx,
		pool:                       pool,
		fc:                         fc,
		clock:                      fixedClock,
		logger:                     infra.Logger,
		fingerprintHasher:          infra.FingerprintHasher,
		repos:                      repos,
		CheckoutSession:            checkoutUC,
		OTP:                        otpUC,
		PaymentResult:              paymentResultUC,
		SagaExecutor:               sagaExecutor,
		confirmationSagaDefinition: confirmationSagaDefinition,
		cancellationSagaDefinition: cancellationSagaDefinition,
		definitions:                registry,
	}
}

// newWorkerExecutor returns a second saga executor with a distinct WorkerID that
// shares the harness clock, pool, and definition registry (lease fencing tests).
func (h *confirmSagaHarness) newWorkerExecutor(workerID string) *sagaexecutoruc.UseCase {
	h.t.Helper()

	sibling := sagaexecutoruc.New(
		h.pool,
		h.repos.Saga,
		h.definitions,
		h.clock,
		workerID,
		h.SagaExecutor.BatchSize,
		h.SagaExecutor.LeaseDuration,
		h.SagaExecutor.PersistenceTimeout,
	)
	require.NoError(h.t, sibling.Validate())

	return sibling
}

func (h *confirmSagaHarness) countContractsForDecision(decisionID model.DecisionID) int {
	h.t.Helper()

	var count int
	err := h.pool.Conn(h.ctx, func(ctx context.Context, conn repo.Conn) error {
		contractQ := h.repos.Contract.Conn(conn)
		_, getErr := contractQ.GetByDecisionID(ctx, decisionID)
		if getErr == nil {
			count = 1
			return nil
		}

		if cerr.IsCode(getErr, cerr.CodeResourceNotFound) || cerr.IsCode(getErr, cerr.CodeContractNotFound) {
			count = 0
			return nil
		}

		return getErr
	})
	require.NoError(h.t, err)

	return count
}

func (h *confirmSagaHarness) advanceClock(d time.Duration) {
	h.clock.Time = h.clock.Time.Add(d)
}

func (h *confirmSagaHarness) findSagaID(sessionID model.CheckoutSessionID) model.SagaID {
	h.t.Helper()

	session := h.querySession(sessionID)
	require.NotNil(h.t, session.ConfirmationAttemptID)
	require.False(h.t, session.ConfirmationAttemptID.IsZero())

	var sagaID model.SagaID
	err := h.pool.Conn(h.ctx, func(ctx context.Context, conn repo.Conn) error {
		sagaQ := h.repos.Saga.Conn(conn)
		instance, getErr := sagaQ.GetByTypeAndBusinessKey(
			ctx,
			model.SagaType("checkout_confirmation"),
			model.SagaBusinessKey(session.ConfirmationAttemptID.String()),
		)
		if getErr != nil {
			return getErr
		}

		sagaID = instance.ID
		return nil
	})
	require.NoError(h.t, err)

	return sagaID
}

func (h *confirmSagaHarness) loadSaga(id model.SagaID) model.SagaInstance {
	h.t.Helper()

	var instance model.SagaInstance
	err := h.pool.Conn(h.ctx, func(ctx context.Context, conn repo.Conn) error {
		sagaQ := h.repos.Saga.Conn(conn)
		row, getErr := sagaQ.GetByID(ctx, id)
		if getErr != nil {
			return getErr
		}

		instance = row
		return nil
	})
	require.NoError(h.t, err)

	return instance
}

func (h *confirmSagaHarness) driveSagaUntilIdle(sagaID model.SagaID) {
	h.t.Helper()
	h.driveWithExecutor(h.SagaExecutor, sagaID)
}

func (h *confirmSagaHarness) driveWithExecutor(executor *sagaexecutoruc.UseCase, sagaID model.SagaID) {
	h.t.Helper()

	const maxSteps = 24
	for i := 0; i < maxSteps; i++ {
		instance := h.loadSaga(sagaID)

		switch instance.Status {
		case model.CompletedSagaStatus,
			model.CompensatedSagaStatus,
			model.FailedSagaStatus,
			model.ReconciliationRequiredSagaStatus,
			model.ManualReviewRequiredSagaStatus:
			return

		case model.WaitingSagaStatus:
			if instance.NextAttemptAt != nil && instance.NextAttemptAt.After(h.clock.Now()) {
				h.advanceClock(instance.NextAttemptAt.Sub(h.clock.Now()) + time.Second)
			}

			err := executor.ProcessDue(h.ctx)
			if err != nil {
				require.NoError(h.t, err)
			}

			continue

		case model.RunningSagaStatus:
			if instance.NextAttemptAt != nil && instance.NextAttemptAt.After(h.clock.Now()) {
				h.advanceClock(instance.NextAttemptAt.Sub(h.clock.Now()) + time.Second)
			}

			err := executor.ExecuteByID(h.ctx, sagaID)
			if err != nil {
				if errors.Is(err, repo.ErrSagaNotRunnable) {
					instance = h.loadSaga(sagaID)
					if instance.NextAttemptAt != nil && instance.NextAttemptAt.After(h.clock.Now()) {
						h.advanceClock(instance.NextAttemptAt.Sub(h.clock.Now()) + time.Second)
						continue
					}

					return
				}

				if errors.Is(err, repo.ErrSagaVersionConflict) {
					return
				}

				require.NoError(h.t, err)
			}

		default:
			return
		}
	}

	h.t.Fatalf("saga %s did not become idle within step budget", sagaID)
}

func (h *confirmSagaHarness) querySession(id model.CheckoutSessionID) model.CheckoutSession {
	h.t.Helper()

	var session model.CheckoutSession
	err := h.pool.Conn(h.ctx, func(ctx context.Context, conn repo.Conn) error {
		sessionQ := h.repos.CheckoutSession.Conn(conn)
		row, getErr := sessionQ.GetByID(ctx, id)
		if getErr != nil {
			return getErr
		}

		session = row
		return nil
	})
	require.NoError(h.t, err)

	return session
}

func (h *confirmSagaHarness) queryOTPChallenge(sessionID model.CheckoutSessionID) model.OTPChallenge {
	h.t.Helper()

	var challenge model.OTPChallenge
	err := h.pool.Conn(h.ctx, func(ctx context.Context, conn repo.Conn) error {
		otpQ := h.repos.OTP.Conn(conn)
		row, getErr := otpQ.GetByPurposeSubject(
			ctx,
			model.OTPPurposeHostedCheckoutConfirm,
			sessionID.String(),
		)
		if getErr != nil {
			return getErr
		}

		challenge = row

		return nil
	})
	require.NoError(h.t, err)

	return challenge
}

func (h *confirmSagaHarness) queryContractBySession(session model.CheckoutSession) model.Contract {
	h.t.Helper()

	require.NotNil(h.t, session.ContractID)

	var contractRow model.Contract
	err := h.pool.Conn(h.ctx, func(ctx context.Context, conn repo.Conn) error {
		contractQ := h.repos.Contract.Conn(conn)
		row, getErr := contractQ.GetByID(ctx, *session.ContractID)
		if getErr != nil {
			return getErr
		}

		contractRow = row
		return nil
	})
	require.NoError(h.t, err)

	return contractRow
}

func (h *confirmSagaHarness) resumeUntilSessionCompleted(sessionID model.CheckoutSessionID) {
	h.t.Helper()

	session := h.querySession(sessionID)
	if session.Status == model.CompletedCheckoutSessionStatus {
		return
	}

	// Simulated process restart: stale confirming recovery runs after the claim
	// timeout; partial saga steps release their executor lease after lease_duration.
	h.advanceClock(checkoutsessionuc.DefaultConfirmingClaimTimeout + time.Minute)
	h.recoverStale()

	const maxSteps = 16
	const sagaLeaseClear = 3 * time.Minute

	for step := 0; step < maxSteps; step++ {
		session = h.querySession(sessionID)
		if session.Status == model.CompletedCheckoutSessionStatus {
			return
		}

		sagaID := h.findSagaID(sessionID)
		instance := h.loadSaga(sagaID)

		switch instance.Status {
		case model.WaitingSagaStatus:
			if instance.NextAttemptAt != nil && instance.NextAttemptAt.After(h.clock.Now()) {
				h.advanceClock(instance.NextAttemptAt.Sub(h.clock.Now()) + time.Second)
			}

			err := h.SagaExecutor.ProcessDue(h.ctx)
			if err != nil {
				require.NoError(h.t, err)
			}

		default:
			h.driveSagaUntilIdle(sagaID)
		}

		h.advanceClock(sagaLeaseClear)
	}

	session = h.querySession(sessionID)
	h.t.Fatalf("session %s did not complete, last status %s", sessionID, session.Status)
}

func (h *confirmSagaHarness) recoverStale() checkoutsessionuc.RecoverStaleConfirmingOutput {
	h.t.Helper()

	output, err := h.CheckoutSession.RecoverStaleConfirmingSessions(h.ctx)
	if err != nil && !cerr.IsCode(err, cerr.CodeServiceUnavailable) {
		require.NoError(h.t, err)
	}

	return output
}
