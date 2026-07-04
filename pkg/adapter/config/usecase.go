package config

import (
	"fmt"
	"time"

	corephonenumber "github.com/parstis/bnpl-backend/pkg/core/canonicalizer/phonenumber"
	"github.com/parstis/bnpl-backend/pkg/core/clock"
	"github.com/parstis/bnpl-backend/pkg/core/domaincapability"
	extcheckoutpayment "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
	extcontractcancellation "github.com/parstis/bnpl-backend/pkg/core/external/contractcancellation"
	extcreditassessment "github.com/parstis/bnpl-backend/pkg/core/external/creditassessment"
	extcustomer "github.com/parstis/bnpl-backend/pkg/core/external/customer"
	extnotification "github.com/parstis/bnpl-backend/pkg/core/external/notification"
	"github.com/parstis/bnpl-backend/pkg/core/fingerprint"
	corehealth "github.com/parstis/bnpl-backend/pkg/core/health"
	"github.com/parstis/bnpl-backend/pkg/core/identifier"
	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/messaging"
	coremetrics "github.com/parstis/bnpl-backend/pkg/core/metrics"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
	"github.com/parstis/bnpl-backend/pkg/core/policy/initialcreditlimit"
	otppolicy "github.com/parstis/bnpl-backend/pkg/core/policy/otp"
	returndestination "github.com/parstis/bnpl-backend/pkg/core/policy/returndestination"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/saga"
	"github.com/parstis/bnpl-backend/pkg/core/secretbox"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/checkoutsessionuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/contractuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/customercredituc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/decisionuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/financialcorepaymentreconciliationuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/financialcorepaymentresultuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/financialcorepaymentsessionuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/healthcheckuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/installmentlifecycleuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/installmentrepaymentuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/migrationsuc"
	otpusecase "github.com/parstis/bnpl-backend/pkg/core/usecase/otp"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/outboxpublisheruc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/paymentattemptreconciliationuc"
)

// Usecases groups configuration sections used to build application-layer
// use cases.
type Usecases struct {
	// Migrations configures filesystem-backed migration settings.
	Migrations Migrations `yaml:"migration"`

	// OutboxPublisher configures outbox publishing use case settings.
	OutboxPublisher OutboxPublisher `yaml:"outbox_publisher"`

	// Health configures Kubernetes-compatible health probe settings.
	Health Health `yaml:"health"`

	// Decision configures the checkout decision use case.
	Decision Decision `yaml:"decision"`

	// CheckoutSession configures merchant checkout handoff lifetime and hosted URL.
	CheckoutSession CheckoutSession `yaml:"checkout_session"`

	// Installment configures installment lifecycle and repayment settings.
	Installment Installment `yaml:"installment"`

	// Contract configures the checkout contract use case.
	Contract Contract `yaml:"contract"`

	// CustomerCredit configures the customer credit use case.
	CustomerCredit CustomerCredit `yaml:"customer_credit_limit"`

	// PaymentAttemptReconciliation configures the payment attempt reconciliation job.
	PaymentAttemptReconciliation PaymentAttemptReconciliation `yaml:"payment_attempt_reconciliation"`

	// FinancialCorePaymentReconciliation configures inbound reconciliation result processing.
	FinancialCorePaymentReconciliation FinancialCorePaymentReconciliation `yaml:"financial_core_payment_reconciliation"`

	// MerchantTerminal configures external partner terminal authentication.
	MerchantTerminal MerchantTerminal `yaml:"merchant_terminal"`

	// Sagas centralizes durable Saga executor and domain-saga behavioral settings.
	Sagas Sagas `yaml:"sagas"`
}

// Migrations contains filesystem-backed migration settings.
type Migrations struct {
	// FilesDir is the filesystem path containing ordered .sql migration files.
	FilesDir string `yaml:"files-dir"`

	// LockTimeout bounds waiting for another migration process holding the
	// database-wide advisory lock. Zero uses the use-case default.
	LockTimeout time.Duration `yaml:"lock_timeout"`
}

func (m Migrations) newUseCase(
	p repo.Pool,
	r repo.Migrations,
	clk clock.Clock,
	logger log.Logger,
) (*migrationsuc.UseCase, error) {
	uc, err := migrationsuc.New(
		p,
		r,
		clk,
		m.FilesDir,
		logger,
	)
	if err != nil {
		return nil, err
	}

	return uc.WithLockTimeout(m.LockTimeout), nil
}

// OutboxPublisher contains outbox publishing use case settings.
type OutboxPublisher struct {
	// WorkerID is the identity used when claiming outbox rows for publishing.
	WorkerID string `yaml:"worker_id"`

	// BatchSize configures how many outbox rows are claimed per publish cycle.
	BatchSize int `yaml:"batch_size"`

	// RetryBaseDelay configures the initial delay before retrying a failed publish.
	RetryBaseDelay time.Duration `yaml:"retry_base_delay"`

	// RetryMaxDelay configures the maximum delay between publish retries.
	RetryMaxDelay time.Duration `yaml:"retry_max_delay"`

	// ProcessingLeaseTimeout configures how long a claimed outbox row remains leased.
	ProcessingLeaseTimeout time.Duration `yaml:"processing_lease_timeout"`
}

func (obp OutboxPublisher) newUseCase(
	pool repo.Pool,
	outbox repo.Outbox,
	publisher messaging.Publisher,
	clock clock.Clock,
	logger log.Logger,
) *outboxpublisheruc.UseCase {
	return outboxpublisheruc.New(
		pool,
		outbox,
		publisher,
		clock,
		logger,
		obp.WorkerID,
		obp.BatchSize,
		obp.RetryBaseDelay,
		obp.RetryMaxDelay,
		obp.ProcessingLeaseTimeout,
	)
}

// Health configures Kubernetes-compatible health probe endpoints.
type Health struct {
	// Enabled controls whether Kubernetes-compatible health probe endpoints are exposed.
	Enabled bool `yaml:"enabled"`
}

func (Health) newUseCase(
	lifecycleState corehealth.LifecycleState,
	dependencyChecker corehealth.DependencyChecker,
) *healthcheckuc.UseCase {
	return healthcheckuc.New(
		lifecycleState,
		dependencyChecker,
	)
}

// Decision configures the checkout decision use case.
type Decision struct{}

func (Decision) newUseCase(
	pool repo.Pool,
	decision repo.Decision,
	customerProfile repo.CustomerProfile,
	creditAccount repo.CreditAccount,
	merchantProfile repo.MerchantProfile,
	merchantBranchProfile repo.MerchantBranchProfile,
	checkoutDecisionAllocation repo.CheckoutDecisionAllocation,
	policyAssignment repo.PolicyAssignment,
	installment repo.Installment,
	installmentCharge repo.InstallmentCharge,
	outbox repo.Outbox,
	clk clock.Clock,
	fingerprintHasher fingerprint.Hasher,
	checkoutPolicies *checkout.Registry,
	defaultCheckoutPolicyRef policy.Ref,
	auditDestination string,
	idGenerator identifier.Generator,
	jurisdictions domaincapability.OriginationJurisdictions,
	businessMetrics coremetrics.BusinessMetrics,
) (*decisionuc.UseCase, error) {
	return decisionuc.New(
		pool,
		decision,
		customerProfile,
		creditAccount,
		merchantProfile,
		merchantBranchProfile,
		checkoutDecisionAllocation,
		policyAssignment,
		installment,
		installmentCharge,
		outbox,
		clk,
		fingerprintHasher,
		checkoutPolicies,
		defaultCheckoutPolicyRef,
		auditDestination,
		idGenerator,
		jurisdictions,
		businessMetrics,
	)
}

// CheckoutSession configures merchant checkout handoffs.
type CheckoutSession struct {
	// PublicOrigin is the externally reachable origin hosting the BNPL checkout
	// UI (scheme://host[:port]). It intentionally excludes paths, query strings,
	// and fragments. Path semantics stay in application code
	// (/checkout/{checkout_session_id}).
	PublicOrigin string `yaml:"public_origin"`

	// PublicOriginPolicy constrains which hosts PublicOrigin may use. Allowed
	// hosts are independent of the origin ENV value under validation.
	PublicOriginPolicy PublicOriginPolicyConfig `yaml:"public_origin_policy"`

	// TTL controls how long a newly-created open checkout session remains usable.
	TTL time.Duration `yaml:"ttl"`

	// ExpiryBatchSize bounds how many open sessions one expiry sweep processes.
	ExpiryBatchSize int `yaml:"expiry_batch_size"`

	// LifecycleEventsDestination is the Kafka topic for merchant checkout
	// lifecycle outbox events (completed/canceled/expired/unavailable).
	LifecycleEventsDestination string `yaml:"lifecycle_events_destination"`

	// WebhookSecretEncryptionActiveKeyID selects the AES-GCM key that encrypts
	// new merchant webhook signing secrets.
	WebhookSecretEncryptionActiveKeyID string `yaml:"webhook_secret_encryption_active_key_id"`

	// WebhookSecretEncryptionKeys are all AES-256 keys (base64, 32 bytes) accepted
	// for decrypting stored webhook signing secrets, including the active one.
	WebhookSecretEncryptionKeys KeyEntries `yaml:"webhook_secret_encryption_keys"`

	// CustomerPhoneEncryptionActiveKeyID selects the AES-GCM key that encrypts
	// the customer phone supplied on external partner checkout creation.
	CustomerPhoneEncryptionActiveKeyID string `yaml:"customer_phone_encryption_active_key_id"`

	// CustomerPhoneEncryptionKeys are all AES-256 keys (base64, 32 bytes) accepted
	// for decrypting stored customer phones, including the active one.
	CustomerPhoneEncryptionKeys KeyEntries `yaml:"customer_phone_encryption_keys"`

	// OTPTemplateID is the Identity Service templated-SMS template for OTP delivery.
	// Empty means DefaultCheckoutOTPTemplateID.
	OTPTemplateID string `yaml:"otp_template_id"`

	// OTPActivePepperKeyID selects the pepper used when hashing a newly issued OTP.
	OTPActivePepperKeyID string `yaml:"otp_active_pepper_key_id"`

	// OTPPepperKeys is the versioned pepper ring for OTP HMAC verifiers.
	OTPPepperKeys KeyEntries `yaml:"otp_pepper_keys"`
}

func (c CheckoutSession) newConfirmationSagaDefinition(
	pool repo.Pool,
	checkoutSession repo.CheckoutSession,
	merchantProfile repo.MerchantProfile,
	customerProfile repo.CustomerProfile,
	contract repo.Contract,
	paymentAttempt repo.PaymentAttempt,
	sagas repo.Saga,
	contractUC *contractuc.UseCase,
	outbox repo.Outbox,
	idGenerator identifier.Generator,
	clk clock.Clock,
	sagaConfig CheckoutConfirmationSaga,
) (*checkoutsessionuc.ConfirmationSagaDefinition, error) {
	const op = "config.CheckoutSession.newConfirmationSagaDefinition"

	lifecycle, err := checkoutsessionuc.NewLifecycleOutbox(
		checkoutSession,
		merchantProfile,
		outbox,
		idGenerator,
		c.LifecycleEventsDestination,
	)
	if err != nil {
		return nil, fmt.Errorf("%s: create lifecycle outbox: %w", op, err)
	}

	sagaConfig.ApplyDefaults()

	return checkoutsessionuc.NewConfirmationSagaDefinition(
		pool,
		checkoutSession,
		customerProfile,
		contract,
		paymentAttempt,
		sagas,
		contractUC,
		lifecycle,
		clk,
		sagaConfig.RetryBaseDelay,
		sagaConfig.RetryMaxDelay,
		sagaConfig.MaxRetryAttempts,
		checkoutsessionuc.ConfirmationSagaHooks{},
	)
}

func (c CheckoutSession) newUseCase(
	pool repo.Pool,
	checkoutSession repo.CheckoutSession,
	merchantProfile repo.MerchantProfile,
	merchantBranchProfile repo.MerchantBranchProfile,
	customerProfile repo.CustomerProfile,
	contract repo.Contract,
	paymentAttempt repo.PaymentAttempt,
	installment repo.Installment,
	installmentCharge repo.InstallmentCharge,
	decisionRepo repo.Decision,
	checkoutDecisionAllocation repo.CheckoutDecisionAllocation,
	checkoutPolicies *checkout.Registry,
	decision *decisionuc.UseCase,
	contractUC *contractuc.UseCase,
	outbox repo.Outbox,
	idGenerator identifier.Generator,
	webhookSecretBox secretbox.Box,
	customerPhoneBox secretbox.Box,
	returnURLPolicy returndestination.Policy,
	clk clock.Clock,
	customerResolver extcustomer.Resolver,
	jurisdictions domaincapability.OriginationJurisdictions,
	phoneCanonicalizer corephonenumber.Canonicalizer,
	env model.RuntimeEnvironment,
	otpUC *otpusecase.UseCase,
	confirmationSagaRepo repo.Saga,
	confirmationSagaExecutor saga.Executor,
	confirmationSagaDefinition *checkoutsessionuc.ConfirmationSagaDefinition,
) (*checkoutsessionuc.UseCase, error) {
	const op = "config.CheckoutSession.newUseCase"

	_, checkoutURLs, err := c.resolvePublicOrigin(env)
	if err != nil {
		return nil, fmt.Errorf("%s: resolve checkout public origin: %w", op, err)
	}

	otpHasher, err := c.NewOTPHasher()
	if err != nil {
		return nil, fmt.Errorf("%s: create otp hasher: %w", op, err)
	}

	return checkoutsessionuc.New(
		pool,
		checkoutSession,
		merchantProfile,
		merchantBranchProfile,
		customerProfile,
		contract,
		paymentAttempt,
		installment,
		installmentCharge,
		decisionRepo,
		checkoutDecisionAllocation,
		checkoutPolicies,
		decision,
		contractUC,
		outbox,
		idGenerator,
		c.LifecycleEventsDestination,
		webhookSecretBox,
		customerPhoneBox,
		returnURLPolicy,
		clk,
		checkoutURLs,
		c.TTL,
		c.ExpiryBatchSize,
		otpHasher,
		customerResolver,
		jurisdictions,
		phoneCanonicalizer,
		otpUC,
		c.EffectiveOTPTemplateID(),
		confirmationSagaRepo,
		confirmationSagaExecutor,
		confirmationSagaDefinition,
	)
}

// newOTPUseCase constructs the reusable OTP use case from checkout-session OTP
// pepper/template settings and shared infrastructure.
func (c CheckoutSession) newOTPUseCase(
	pool repo.Pool,
	challenges repo.OTP,
	notificationSubmitter extnotification.Submitter,
	clk clock.Clock,
) (*otpusecase.UseCase, error) {
	const op = "config.CheckoutSession.newOTPUseCase"

	otpHasher, err := c.NewOTPHasher()
	if err != nil {
		return nil, fmt.Errorf("%s: create otp hasher: %w", op, err)
	}

	return otpusecase.New(
		pool,
		challenges,
		otpHasher,
		notificationSubmitter,
		otppolicy.V1(),
		clk,
		nil,
	)
}

// Installment configures installment lifecycle batch sizes and repayment
// outbox destination. Checkout installment offer creation is owned by
// decisionuc (POST /api/v1/checkout-decisions), not a standalone use case.
//
// YAML path was renamed from usecases.checkout_installment_offer to
// usecases.installment. Environment variable names are unchanged.
type Installment struct {
	// MarkDueBatchSize configures how many installments are marked due per batch.
	MarkDueBatchSize int `yaml:"mark_due_batch_size"`

	// MarkOverdueBatchSize configures how many installments are marked overdue per batch.
	MarkOverdueBatchSize int `yaml:"mark_overdue_batch_size"`

	// InstallmentPaymentRequestsDestination is the Kafka destination for installment
	// payment request outbox messages.
	InstallmentPaymentRequestsDestination string `yaml:"installment_payment_requests_destination"`
}

func (i Installment) newLifecycleUseCase(
	pool repo.Pool,
	installment repo.Installment,
	installmentCharge repo.InstallmentCharge,
	outbox repo.Outbox,
	clk clock.Clock,
	auditDestination string,
	idGenerator identifier.Generator,
	businessMetrics coremetrics.BusinessMetrics,
) (*installmentlifecycleuc.UseCase, error) {
	return installmentlifecycleuc.New(
		pool,
		installment,
		installmentCharge,
		outbox,
		clk,
		auditDestination,
		idGenerator,
		i.MarkDueBatchSize,
		i.MarkOverdueBatchSize,
		businessMetrics,
	)
}

func (i Installment) newRepaymentUseCase(
	pool repo.Pool,
	customerProfile repo.CustomerProfile,
	merchantProfile repo.MerchantProfile,
	contractRepo repo.Contract,
	installment repo.Installment,
	installmentCharge repo.InstallmentCharge,
	paymentAttempt repo.PaymentAttempt,
	outbox repo.Outbox,
	checkoutPaymentClient extcheckoutpayment.Client,
	clk clock.Clock,
	logger log.Logger,
	auditDestination string,
	idGenerator identifier.Generator,
	paymentReturnURL string,
) (*installmentrepaymentuc.UseCase, error) {
	return installmentrepaymentuc.New(
		pool,
		customerProfile,
		merchantProfile,
		contractRepo,
		installment,
		installmentCharge,
		paymentAttempt,
		outbox,
		checkoutPaymentClient,
		clk,
		logger,
		auditDestination,
		idGenerator,
		paymentReturnURL,
	)
}

// Contract configures the checkout contract use case.
type Contract struct {
	// PaymentResultProcessingGracePeriod configures how long payment results may still be processed after expiry.
	PaymentResultProcessingGracePeriod time.Duration `yaml:"payment_result_processing_grace_period"`

	// ExpiryBatchSize configures how many contracts are expired per batch.
	ExpiryBatchSize int `yaml:"expiry_batch_size"`
}

func (c Contract) newUseCase(
	pool repo.Pool,
	customerProfile repo.CustomerProfile,
	merchantProfile repo.MerchantProfile,
	creditAccount repo.CreditAccount,
	decision repo.Decision,
	contractRepo repo.Contract,
	checkoutDecisionAllocation repo.CheckoutDecisionAllocation,
	checkoutPolicies *checkout.Registry,
	installment repo.Installment,
	installmentCharge repo.InstallmentCharge,
	paymentAttempt repo.PaymentAttempt,
	checkoutPaymentClient extcheckoutpayment.Client,
	contractCancellationClient extcontractcancellation.Client,
	outbox repo.Outbox,
	clk clock.Clock,
	auditDestination string,
	idGenerator identifier.Generator,
	paymentReturnURL string,
	cancellationSagaRepo repo.Saga,
	cancellationSagaExecutor saga.Executor,
	cancellationSagaDefinition saga.Definition,
	jurisdictions domaincapability.OriginationJurisdictions,
	logger log.Logger,
	businessMetrics coremetrics.BusinessMetrics,
	sagaConfig ContractCancellationSaga,
) (*contractuc.UseCase, error) {
	sagaConfig.ApplyDefaults()

	return contractuc.New(
		pool,
		customerProfile,
		merchantProfile,
		creditAccount,
		decision,
		contractRepo,
		checkoutDecisionAllocation,
		checkoutPolicies,
		installment,
		installmentCharge,
		paymentAttempt,
		checkoutPaymentClient,
		contractCancellationClient,
		outbox,
		clk,
		auditDestination,
		idGenerator,
		paymentReturnURL,
		c.PaymentResultProcessingGracePeriod,
		c.ExpiryBatchSize,
		sagaConfig.ToUseCaseSettings(),
		cancellationSagaRepo,
		cancellationSagaExecutor,
		cancellationSagaDefinition,
		jurisdictions,
		logger,
		businessMetrics,
	)
}

func newCancellationSagaDefinition(
	pool repo.Pool,
	contractRepo repo.Contract,
	creditAccount repo.CreditAccount,
	client extcontractcancellation.Client,
	clk clock.Clock,
	sagaConfig ContractCancellationSaga,
) (saga.Definition, error) {
	sagaConfig.ApplyDefaults()
	return contractuc.NewCancellationSagaDefinition(
		pool,
		contractRepo,
		creditAccount,
		client,
		clk,
		sagaConfig.RetryBaseDelay,
		sagaConfig.RetryMaxDelay,
	)
}

func (c Contract) newFinancialCorePaymentResultUseCase(
	pool repo.Pool,
	contractUC *contractuc.UseCase,
	checkoutSessionUC *checkoutsessionuc.UseCase,
	checkoutPaymentClient extcheckoutpayment.Client,
	inbox repo.Inbox,
	outbox repo.Outbox,
	paymentAttempt repo.PaymentAttempt,
	contractRepo repo.Contract,
	installment repo.Installment,
	installmentCharge repo.InstallmentCharge,
	creditAccount repo.CreditAccount,
	idGenerator identifier.Generator,
	fingerprintHasher fingerprint.Hasher,
	paymentApplicationResultsDestination string,
	auditDestination string,
	clk clock.Clock,
	businessMetrics coremetrics.BusinessMetrics,
) (*financialcorepaymentresultuc.UseCase, error) {
	return financialcorepaymentresultuc.New(
		pool,
		contractUC,
		checkoutSessionUC,
		checkoutPaymentClient,
		inbox,
		outbox,
		paymentAttempt,
		contractRepo,
		installment,
		installmentCharge,
		creditAccount,
		idGenerator,
		fingerprintHasher,
		paymentApplicationResultsDestination,
		auditDestination,
		clk,
		c.PaymentResultProcessingGracePeriod,
		businessMetrics,
	)
}

func (Contract) newFinancialCorePaymentSessionUseCase(
	pool repo.Pool,
	inbox repo.Inbox,
	paymentAttempt repo.PaymentAttempt,
	outbox repo.Outbox,
	idGenerator identifier.Generator,
	fingerprintHasher fingerprint.Hasher,
	auditDestination string,
	clk clock.Clock,
) (*financialcorepaymentsessionuc.UseCase, error) {
	return financialcorepaymentsessionuc.New(
		pool,
		inbox,
		paymentAttempt,
		outbox,
		idGenerator,
		fingerprintHasher,
		auditDestination,
		clk,
	)
}

// CustomerCredit configures the customer credit use case.
type CustomerCredit struct{}

func (CustomerCredit) newUseCase(
	pool repo.Pool,
	customerProfile repo.CustomerProfile,
	creditAccount repo.CreditAccount,
	decision repo.Decision,
	outbox repo.Outbox,
	clk clock.Clock,
	fingerprintHasher fingerprint.Hasher,
	creditAssessmentClient extcreditassessment.Client,
	policyAssignment repo.PolicyAssignment,
	initialLimitPolicies *initialcreditlimit.Registry,
	defaultInitialLimitRef policy.Ref,
	auditDestination string,
	idGenerator identifier.Generator,
) (*customercredituc.UseCase, error) {
	return customercredituc.New(
		pool,
		customerProfile,
		creditAccount,
		decision,
		outbox,
		clk,
		fingerprintHasher,
		creditAssessmentClient,
		policyAssignment,
		initialLimitPolicies,
		defaultInitialLimitRef,
		auditDestination,
		idGenerator,
	)
}

// PaymentAttemptReconciliation configures the payment attempt reconciliation job.
type PaymentAttemptReconciliation struct {
	// BatchSize configures how many payment attempts are reconciled per cycle.
	BatchSize int `yaml:"batch_size"`

	// CreatedTimeout configures how long a created payment attempt may remain unresolved.
	CreatedTimeout time.Duration `yaml:"created_timeout"`

	// SubmittedExpiryGrace configures the grace period after a submitted attempt expires.
	SubmittedExpiryGrace time.Duration `yaml:"submitted_expiry_grace"`

	// RetryBaseDelay configures the initial delay before retrying reconciliation.
	RetryBaseDelay time.Duration `yaml:"retry_base_delay"`

	// RetryMaxDelay configures the maximum delay between reconciliation retries.
	RetryMaxDelay time.Duration `yaml:"retry_max_delay"`

	// MaxAttempts configures the maximum number of reconciliation attempts per payment.
	MaxAttempts int `yaml:"max_attempts"`
}

func (p PaymentAttemptReconciliation) newUseCase(
	pool repo.Pool,
	paymentAttempt repo.PaymentAttempt,
	contractRepo repo.Contract,
	outbox repo.Outbox,
	idGenerator identifier.Generator,
	reconciliationRequestsDestination string,
	auditDestination string,
	clk clock.Clock,
) (*paymentattemptreconciliationuc.UseCase, error) {
	maxAttempts := p.MaxAttempts
	if maxAttempts == 0 {
		maxAttempts = 10
	}

	return paymentattemptreconciliationuc.New(
		pool,
		paymentAttempt,
		contractRepo,
		outbox,
		idGenerator,
		reconciliationRequestsDestination,
		auditDestination,
		p.BatchSize,
		p.CreatedTimeout,
		p.SubmittedExpiryGrace,
		p.RetryBaseDelay,
		p.RetryMaxDelay,
		maxAttempts,
		clk,
	)
}

// FinancialCorePaymentReconciliation configures inbound reconciliation result processing.
type FinancialCorePaymentReconciliation struct {
	// RetryBaseDelay configures the initial delay before retrying inbound reconciliation processing.
	RetryBaseDelay time.Duration `yaml:"retry_base_delay"`

	// RetryMaxDelay configures the maximum delay between inbound reconciliation retries.
	RetryMaxDelay time.Duration `yaml:"retry_max_delay"`
}

// effectiveRetryDelays returns this section's retry delays, falling back to
// fallback's delays when a value is not configured (<= 0).
//
// The fallback exists so that deployments which only configure
// payment_attempt_reconciliation retry timing do not need to duplicate the
// same values under financial_core_payment_reconciliation.
func (f FinancialCorePaymentReconciliation) effectiveRetryDelays(
	fallback PaymentAttemptReconciliation,
) (retryBaseDelay, retryMaxDelay time.Duration) {
	retryBaseDelay = f.RetryBaseDelay
	if retryBaseDelay <= 0 {
		retryBaseDelay = fallback.RetryBaseDelay
	}

	retryMaxDelay = f.RetryMaxDelay
	if retryMaxDelay <= 0 {
		retryMaxDelay = fallback.RetryMaxDelay
	}

	return retryBaseDelay, retryMaxDelay
}

func (FinancialCorePaymentReconciliation) newUseCase(
	pool repo.Pool,
	paymentResult *financialcorepaymentresultuc.UseCase,
	inbox repo.Inbox,
	outbox repo.Outbox,
	paymentAttempt repo.PaymentAttempt,
	contractRepo repo.Contract,
	installment repo.Installment,
	installmentCharge repo.InstallmentCharge,
	creditAccount repo.CreditAccount,
	fingerprintHasher fingerprint.Hasher,
	idGenerator identifier.Generator,
	installmentPaymentReconciliationAcknowledgement string,
	auditDestination string,
	retryBaseDelay time.Duration,
	retryMaxDelay time.Duration,
	clk clock.Clock,
	logger log.Logger,
) (*financialcorepaymentreconciliationuc.UseCase, error) {
	return financialcorepaymentreconciliationuc.New(
		pool,
		paymentResult,
		inbox,
		outbox,
		paymentAttempt,
		contractRepo,
		installment,
		installmentCharge,
		creditAccount,
		fingerprintHasher,
		idGenerator,
		installmentPaymentReconciliationAcknowledgement,
		auditDestination,
		retryBaseDelay,
		retryMaxDelay,
		clk,
		logger,
	)
}
