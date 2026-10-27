package config

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/migrationsrp"
	adapterbotmessage "github.com/parstis/bnpl-backend/pkg/adapter/external/botmessage"
	fingerprinthmac "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/fingerprint/hmac"
	secretboxadapter "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/secretbox"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin"
	"github.com/parstis/bnpl-backend/pkg/core/cache"
	"github.com/parstis/bnpl-backend/pkg/core/circuitbreaker"
	"github.com/parstis/bnpl-backend/pkg/core/clock"
	"github.com/parstis/bnpl-backend/pkg/core/domaincapability"
	coreerrorreporting "github.com/parstis/bnpl-backend/pkg/core/errorreporting"
	extcheckoutpayment "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
	extcontractcancellation "github.com/parstis/bnpl-backend/pkg/core/external/contractcancellation"
	extcreditassessment "github.com/parstis/bnpl-backend/pkg/core/external/creditassessment"
	extcustomer "github.com/parstis/bnpl-backend/pkg/core/external/customer"
	extinstallmentpayment "github.com/parstis/bnpl-backend/pkg/core/external/installmentpayment"
	extnotification "github.com/parstis/bnpl-backend/pkg/core/external/notification"
	"github.com/parstis/bnpl-backend/pkg/core/fingerprint"
	corehealth "github.com/parstis/bnpl-backend/pkg/core/health"
	corehttpclient "github.com/parstis/bnpl-backend/pkg/core/httpclient"
	"github.com/parstis/bnpl-backend/pkg/core/identifier"
	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/messaging"
	coremetrics "github.com/parstis/bnpl-backend/pkg/core/metrics"
	coreobjectstorage "github.com/parstis/bnpl-backend/pkg/core/objectstorage"
	"github.com/parstis/bnpl-backend/pkg/core/policy"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
	"github.com/parstis/bnpl-backend/pkg/core/policy/initialcreditlimit"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/saga"
	"github.com/parstis/bnpl-backend/pkg/core/secretbox"
	"github.com/parstis/bnpl-backend/pkg/core/task"
	"github.com/parstis/bnpl-backend/pkg/core/trace"
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
	"github.com/parstis/bnpl-backend/pkg/core/usecase/merchantterminaluc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/migrationsuc"
	otpusecase "github.com/parstis/bnpl-backend/pkg/core/usecase/otp"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/outboxpublisheruc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/paymentattemptreconciliationuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/sagaexecutoruc"
)

// Noop holds configuration for the noop cache provider.
//
// The noop provider currently has no configurable options. The struct exists
// to keep the configuration schema consistent across cache providers and to
// allow future extensions without breaking compatibility.
type Noop struct{}

// Config is the root runtime configuration document.
// It aggregates adapter and application settings loaded from YAML and
// environment variable expansion. Callers typically load it once at startup
// and treat it as immutable afterwards.
type Config struct {
	// Runtime contains root deployment runtime settings.
	Runtime Runtime `yaml:"runtime"`

	// Database contains PostgreSQL connection and pool settings.
	Database Database

	// Gin contains HTTP server settings for the Gin adapter.
	Gin Gin

	// Domain configures deployment-enabled domain capabilities such as
	// jurisdictions. It is not infrastructure or use-case wiring.
	Domain Domain `yaml:"domain"`

	// Usecases contains application-layer use case settings.
	Usecases Usecases

	// Infrastructure contains runtime infrastructure settings.
	Infrastructure Infrastructure

	// External contains configuration for business-facing external systems.
	External External

	// Versions contains the configuration file and database schema version
	// strings corresponding to this Config instance and its Database target.
	Versions Versions
}

// Load reads a YAML configuration file from path, expands ${VAR} placeholders
// from the current process environment, and decodes the result into Config.
// It returns an error when the file cannot be read, the YAML document shape is
// unexpected, or decoding fails. The function is concurrency-safe because it
// only works with local state and process environment reads.
func Load(path string) (*Config, error) {
	const op = "config.Load"

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%s: reading config file: %w", op, err)
	}

	n := &yaml.Node{}
	expandedEnv := os.ExpandEnv(string(data))
	if err = yaml.Unmarshal([]byte(expandedEnv), n); err != nil {
		return nil, fmt.Errorf("%s: unmarshalling yaml: %w", op, err)
	}

	if l := len(n.Content); l != 1 {
		return nil, fmt.Errorf("%s: found %d children nodes, instead of 1 mapping child", op, l)
	}

	c := &Config{}
	if err = n.Decode(c); err != nil {
		return nil, fmt.Errorf("%s: decoding yaml node: %w", op, err)
	}

	if err = c.ValidateVersions(); err != nil {
		return nil, fmt.Errorf("%s: validating versions: %w", op, err)
	}

	if err = c.Validate(); err != nil {
		return nil, fmt.Errorf("%s: validating configuration: %w", op, err)
	}

	return c, nil
}

// ValidateVersions checks that the versions declared in the configuration
// file are supported by this binary.
func (c *Config) ValidateVersions() error {
	return c.Versions.Validate()
}

// GetShutdownTimeout returns the configured graceful shutdown timeout.
func (c *Config) GetShutdownTimeout() time.Duration {
	return c.Infrastructure.Shutdown.getShutdownTimeout()
}

// GetShutdownDrainDelay returns the configured graceful shutdown drain delay.
func (c *Config) GetShutdownDrainDelay() time.Duration {
	return c.Infrastructure.Shutdown.getShutdownDrainDelay()
}

// ConnectionPool builds a database connection pool for role.
// The returned pool owns live network connections and must be closed by the
// caller. Pool creation may perform connectivity checks and therefore has
// observable side effects on the target database.
//
// logger is the shared process logger. When database query logging is enabled,
// it must be non-nil; ConnectionPool only derives contextual attributes from it
// and never constructs a new logger.
func (c *Config) ConnectionPool(
	ctx context.Context,
	r repo.Role,
	logger corelog.Logger,
) (repo.Pool, error) {
	var queryLogger corelog.Logger
	const op = "config.Config.ConnectionPool"

	if c.Database.QueryLogging.Enabled {
		if logger == nil {
			return nil, fmt.Errorf(
				"%s: logger is required when database query logging is enabled", op,
			)
		}

		queryLogger = logger.With(
			"component", "postgres",
			"database_role", string(r),
		)
	}

	p, err := c.Database.ConnectionPool(ctx, r, queryLogger)
	if err != nil {
		return nil, fmt.Errorf(
			"%s: %#v.ConnectionPool: %w", op, c.Database, err,
		)
	}

	return p, nil
}

// ConnectionInfo returns the configured database name, host, and port.
// It is a pure accessor and has no side effects.
func (c *Config) ConnectionInfo() (dbName, host string, port int) {
	return c.Database.connectionInfo()
}

// NewSchemaRepo constructs the schema repository adapter configured for this
// process. The returned repository is stateless and safe to reuse.
func (c *Config) NewSchemaRepo() repo.Schema {
	return c.Database.newSchemaRepo()
}

// GetRolePassword resolves the configured password for role from the pgpass
// entries embedded in the configuration. It returns an error when no matching
// entry exists.
func (c *Config) GetRolePassword(r repo.Role) (string, error) {
	return c.Database.getRolePassword(r)
}

// SchemaInitializer creates a schema initializer bound to tx.
// The initializer uses the supplied transaction exclusively; callers must not
// use it after the transaction has been committed or rolled back.
func (c *Config) SchemaInitializer(tx repo.Tx) repo.SchemaInitializer {
	return migrationsrp.NewInitializer(tx)
}

// NewMigrationsUseCase constructs the migration use case configured by this
// repository. The returned use case reuses the caller-provided pool and
// repository and does not take ownership beyond its own method calls.
func (c *Config) NewMigrationsUseCase(
	p repo.Pool,
	r repo.Migrations,
	clk clock.Clock,
	logger corelog.Logger,
) (*migrationsuc.UseCase, error) {
	return c.Usecases.Migrations.newUseCase(p, r, clk, logger)
}

// NewOutboxPublisherUseCase creates the use case responsible for publishing
// integration outbox messages to the configured messaging infrastructure.
func (c *Config) NewOutboxPublisherUseCase(
	pool repo.Pool,
	outbox repo.Outbox,
	publisher messaging.Publisher,
	clock clock.Clock,
	logger corelog.Logger,
) *outboxpublisheruc.UseCase {
	return c.Usecases.OutboxPublisher.newUseCase(
		pool,
		outbox,
		publisher,
		clock,
		logger,
	)
}

// NewCheckoutSessionUseCase constructs the merchant checkout-session use case.
//
// jurisdictions must be the shared composition-root instance. Phone Registry
// is built from that same concrete set so every enabled origination jurisdiction
// has a compiled phone implementation.
func (c *Config) NewOTPUseCase(
	pool repo.Pool,
	challenges repo.OTP,
	notificationSubmitter extnotification.Submitter,
	clk clock.Clock,
) (*otpusecase.UseCase, error) {
	return c.Usecases.CheckoutSession.newOTPUseCase(
		pool,
		challenges,
		notificationSubmitter,
		clk,
	)
}

func (c *Config) NewCheckoutSessionUseCase(
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
	clk clock.Clock,
	customerResolver extcustomer.Resolver,
	jurisdictions *domaincapability.Jurisdictions,
	otpUC *otpusecase.UseCase,
	confirmationSagaRepo repo.Saga,
	confirmationSagaExecutor saga.Executor,
	confirmationSagaDefinition *checkoutsessionuc.ConfirmationSagaDefinition,
) (*checkoutsessionuc.UseCase, error) {
	env, err := c.Runtime.ParsedEnvironment()
	if err != nil {
		return nil, err
	}

	phoneCanonicalizer, err := c.NewPhoneCanonicalizer(jurisdictions)
	if err != nil {
		return nil, err
	}

	return c.Usecases.CheckoutSession.newUseCase(
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
		webhookSecretBox,
		customerPhoneBox,
		returnURLPolicyForEnvironment(env),
		clk,
		customerResolver,
		jurisdictions,
		phoneCanonicalizer,
		env,
		otpUC,
		confirmationSagaRepo,
		confirmationSagaExecutor,
		confirmationSagaDefinition,
	)
}

// NewCheckoutConfirmationSagaDefinition constructs the durable checkout
// confirmation Saga definition owned by the checkout-session domain.
func (c *Config) NewCheckoutConfirmationSagaDefinition(
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
) (*checkoutsessionuc.ConfirmationSagaDefinition, error) {
	return c.Usecases.CheckoutSession.newConfirmationSagaDefinition(
		pool,
		checkoutSession,
		merchantProfile,
		customerProfile,
		contract,
		paymentAttempt,
		sagas,
		contractUC,
		outbox,
		idGenerator,
		clk,
		c.Usecases.Sagas.CheckoutConfirmation,
	)
}

// NewMerchantTerminalHasher constructs the peppered HMAC fingerprint hasher used
// for partner API key digests.
func (c *Config) NewMerchantTerminalHasher() (*fingerprinthmac.Hasher, error) {
	return c.Usecases.MerchantTerminal.NewHasher()
}

// NewMerchantTerminalSecretBox constructs the secret box that protects issued
// merchant API keys held for idempotent replay.
func (c *Config) NewMerchantTerminalSecretBox() (*secretboxadapter.Box, error) {
	return c.Usecases.MerchantTerminal.NewIssuanceSecretBox()
}

// NewMerchantTerminalUseCase constructs the external partner terminal use case.
func (c *Config) NewMerchantTerminalUseCase(
	pool repo.Pool,
	merchantProfile repo.MerchantProfile,
	merchantTerminal repo.MerchantTerminal,
	hasher fingerprint.Hasher,
	box secretbox.Box,
	clk clock.Clock,
) (*merchantterminaluc.UseCase, error) {
	return c.Usecases.MerchantTerminal.newTerminalUseCase(
		pool,
		merchantProfile,
		merchantTerminal,
		hasher,
		box,
		clk,
	)
}

// NewDecisionUseCase constructs the checkout decision use case configured by this process.
//
// jurisdictions must be the shared composition-root origination capability.
func (c *Config) NewDecisionUseCase(
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
	idGenerator identifier.Generator,
	jurisdictions domaincapability.OriginationJurisdictions,
	businessMetrics coremetrics.BusinessMetrics,
	newCreditUsageGuard decisionuc.NewCreditUsageGuard,
) (*decisionuc.UseCase, error) {
	return c.Usecases.Decision.newUseCase(
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
		c.AuditEventsTopic(),
		idGenerator,
		jurisdictions,
		businessMetrics,
		newCreditUsageGuard,
	)
}

// NewInstallmentLifecycleUseCase constructs the installment lifecycle use case.
func (c *Config) NewInstallmentLifecycleUseCase(
	pool repo.Pool,
	installment repo.Installment,
	installmentCharge repo.InstallmentCharge,
	outbox repo.Outbox,
	clk clock.Clock,
	idGenerator identifier.Generator,
	businessMetrics coremetrics.BusinessMetrics,
) (*installmentlifecycleuc.UseCase, error) {
	return c.Usecases.Installment.newLifecycleUseCase(
		pool,
		installment,
		installmentCharge,
		outbox,
		clk,
		c.AuditEventsTopic(),
		idGenerator,
		businessMetrics,
	)
}

// NewInstallmentRepaymentUseCase constructs the installment repayment use case.
func (c *Config) NewInstallmentRepaymentUseCase(
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
	logger corelog.Logger,
	idGenerator identifier.Generator,
) (*installmentrepaymentuc.UseCase, error) {
	return c.Usecases.Installment.newRepaymentUseCase(
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
		c.AuditEventsTopic(),
		idGenerator,
		c.External.FinancialCore.PaymentReturnURL,
	)
}

// NewFinancialCorePaymentSessionUseCase constructs the Financial Core payment-session use case.
func (c *Config) NewFinancialCorePaymentSessionUseCase(
	pool repo.Pool,
	inbox repo.Inbox,
	paymentAttempt repo.PaymentAttempt,
	outbox repo.Outbox,
	idGenerator identifier.Generator,
	fingerprintHasher fingerprint.Hasher,
	clk clock.Clock,
) (*financialcorepaymentsessionuc.UseCase, error) {
	return c.Usecases.Contract.newFinancialCorePaymentSessionUseCase(
		pool,
		inbox,
		paymentAttempt,
		outbox,
		idGenerator,
		fingerprintHasher,
		c.AuditEventsTopic(),
		clk,
	)
}

// NewSagaExecutorUseCase constructs the durable Saga executor over the supplied
// Saga definitions.
//
// The executor stays domain-agnostic: pass every Saga definition that this
// process must be able to resume. Domains own definition construction; this
// factory only builds the registry and executor.
func (c *Config) NewSagaExecutorUseCase(
	pool repo.Pool,
	sagaRepo repo.Saga,
	clk clock.Clock,
	definitions ...saga.Definition,
) (*sagaexecutoruc.UseCase, error) {
	const op = "config.Config.NewSagaExecutorUseCase"

	registry, err := saga.NewRegistry(definitions...)
	if err != nil {
		return nil, fmt.Errorf("%s: initialize saga definition registry: %w", op, err)
	}

	uc := c.Usecases.Sagas.Executor.newUseCase(pool, sagaRepo, registry, clk)
	if err = uc.Validate(); err != nil {
		return nil, err
	}

	return uc, nil
}

// NewContractCancellationSagaDefinition constructs the versioned contract
// cancellation Saga definition owned by the contract domain.
func (c *Config) NewContractCancellationSagaDefinition(
	pool repo.Pool,
	contractRepo repo.Contract,
	creditAccount repo.CreditAccount,
	client extcontractcancellation.Client,
	clk clock.Clock,
) (saga.Definition, error) {
	return newCancellationSagaDefinition(
		pool,
		contractRepo,
		creditAccount,
		client,
		clk,
		c.Usecases.Sagas.ContractCancellation,
	)
}

// NewContractUseCase constructs the checkout contract use case.
//
// cancellationSagaDefinition must be the same definition instance registered
// with the Saga executor for contract cancellation.
//
// jurisdictions must be the shared composition-root origination capability and
// is used only to gate creation of a first Contract for a Decision.
func (c *Config) NewContractUseCase(
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
	idGenerator identifier.Generator,
	cancellationSagaRepo repo.Saga,
	cancellationSagaExecutor saga.Executor,
	cancellationSagaDefinition saga.Definition,
	jurisdictions domaincapability.OriginationJurisdictions,
	logger corelog.Logger,
	businessMetrics coremetrics.BusinessMetrics,
	newCreditUsageGuard contractuc.NewCreditUsageGuard,
) (*contractuc.UseCase, error) {
	return c.Usecases.Contract.newUseCase(
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
		c.AuditEventsTopic(),
		idGenerator,
		c.External.FinancialCore.PaymentReturnURL,
		cancellationSagaRepo,
		cancellationSagaExecutor,
		cancellationSagaDefinition,
		jurisdictions,
		logger,
		businessMetrics,
		c.Usecases.Sagas.ContractCancellation,
		newCreditUsageGuard,
	)
}

// NewFinancialCorePaymentResultUseCase constructs the Financial Core payment-result use case.
func (c *Config) NewFinancialCorePaymentResultUseCase(
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
	clk clock.Clock,
	businessMetrics coremetrics.BusinessMetrics,
) (*financialcorepaymentresultuc.UseCase, error) {
	return c.Usecases.Contract.newFinancialCorePaymentResultUseCase(
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
		c.Infrastructure.Messaging.Kafka.Topics.PaymentApplicationResults,
		c.AuditEventsTopic(),
		clk,
		businessMetrics,
	)
}

// NewPaymentAttemptReconciliationUseCase constructs the payment attempt reconciliation job use case.
func (c *Config) NewPaymentAttemptReconciliationUseCase(
	pool repo.Pool,
	paymentAttempt repo.PaymentAttempt,
	contractRepo repo.Contract,
	outbox repo.Outbox,
	idGenerator identifier.Generator,
	clk clock.Clock,
) (*paymentattemptreconciliationuc.UseCase, error) {
	return c.Usecases.PaymentAttemptReconciliation.newUseCase(
		pool,
		paymentAttempt,
		contractRepo,
		outbox,
		idGenerator,
		c.InstallmentPaymentReconciliationRequestsTopic(),
		c.AuditEventsTopic(),
		clk,
	)
}

// NewFinancialCorePaymentReconciliationUseCase constructs the inbound reconciliation result use case.
func (c *Config) NewFinancialCorePaymentReconciliationUseCase(
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
	clk clock.Clock,
	logger corelog.Logger,
) (*financialcorepaymentreconciliationuc.UseCase, error) {
	retryBaseDelay, retryMaxDelay := c.Usecases.FinancialCorePaymentReconciliation.effectiveRetryDelays(
		c.Usecases.PaymentAttemptReconciliation,
	)

	return c.Usecases.FinancialCorePaymentReconciliation.newUseCase(
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
		c.GetInstallmentPaymentReconciliationAcknowledgementsTopic(),
		c.AuditEventsTopic(),
		retryBaseDelay,
		retryMaxDelay,
		clk,
		logger,
	)
}

// NewCustomerCreditUseCase constructs the customer credit use case.
func (c *Config) NewCustomerCreditUseCase(
	pool repo.Pool,
	customerProfile repo.CustomerProfile,
	creditAccount repo.CreditAccount,
	decision repo.Decision,
	customerRestriction repo.CustomerRestriction,
	installment repo.Installment,
	defaultEpisode repo.DefaultEpisode,
	outbox repo.Outbox,
	clk clock.Clock,
	fingerprintHasher fingerprint.Hasher,
	creditAssessmentClient extcreditassessment.Client,
	policyAssignment repo.PolicyAssignment,
	initialLimitPolicies *initialcreditlimit.Registry,
	defaultInitialLimitRef policy.Ref,
	idGenerator identifier.Generator,
) (*customercredituc.UseCase, error) {
	return c.Usecases.CustomerCredit.newUseCase(
		pool,
		customerProfile,
		creditAccount,
		decision,
		customerRestriction,
		installment,
		defaultEpisode,
		outbox,
		clk,
		fingerprintHasher,
		creditAssessmentClient,
		policyAssignment,
		initialLimitPolicies,
		defaultInitialLimitRef,
		c.AuditEventsTopic(),
		idGenerator,
	)
}

// NewHealthUseCase constructs the health check use case configured by this
// process.
func (c *Config) NewHealthUseCase(
	lifecycleState corehealth.LifecycleState,
	dependencyChecker corehealth.DependencyChecker,
) *healthcheckuc.UseCase {
	return c.Usecases.Health.newUseCase(lifecycleState, dependencyChecker)
}

// NewGinEngine constructs the HTTP engine configured for this process.
//
// logger is the application structured logger used by optional engine-level
// diagnostic middleware such as request debug logging.
func (c *Config) NewGinEngine(
	provider trace.Provider,
	logger corelog.Logger,
	reporter coreerrorreporting.Reporter,
) (*gin.Engine, error) {
	return c.Gin.newEngine(provider, logger, reporter)
}

// GetGinListeningAddress returns the configured HTTP listen address.
func (c *Config) GetGinListeningAddress() string {
	return c.Gin.getListeningAddress()
}

// GetGinReadHeaderTimeout returns the HTTP server read-header timeout.
func (c *Config) GetGinReadHeaderTimeout() time.Duration {
	return c.Gin.getReadHeaderTimeout()
}

// GetGinReadTimeout returns the complete-request read timeout.
func (c *Config) GetGinReadTimeout() time.Duration { return c.Gin.getReadTimeout() }

// GetGinWriteTimeout returns the response write timeout.
func (c *Config) GetGinWriteTimeout() time.Duration { return c.Gin.getWriteTimeout() }

// GetGinIdleTimeout returns the keep-alive idle timeout.
func (c *Config) GetGinIdleTimeout() time.Duration { return c.Gin.getIdleTimeout() }

// GetGinMaxHeaderBytes returns the HTTP request-header size limit.
func (c *Config) GetGinMaxHeaderBytes() int { return c.Gin.getMaxHeaderBytes() }

// NewDependencyCheckerInfrastructure constructs the application's composite dependency
// checker from the configured health checks.
func (c *Config) NewDependencyCheckerInfrastructure(
	pool repo.Pool,
	healthRepo repo.Health,
	cacheInfra cache.Cache,
	messagingHealth messaging.Health,
) (corehealth.DependencyChecker, error) {
	return c.Infrastructure.HealthCheck.newDependencyCheckerInfrastructure(
		pool,
		healthRepo,
		cacheInfra,
		messagingHealth,
	)
}

// NewDependencyCheckerInfrastructureWithConsumer constructs the dependency checker
// including Kafka consumer supervision when configured.
func (c *Config) NewDependencyCheckerInfrastructureWithConsumer(
	pool repo.Pool,
	healthRepo repo.Health,
	cacheInfra cache.Cache,
	messagingHealth messaging.Health,
	consumerHealth corehealth.ConsumerHealth,
) (corehealth.DependencyChecker, error) {
	return c.Infrastructure.HealthCheck.newDependencyCheckerInfrastructureWithConsumer(
		pool,
		healthRepo,
		cacheInfra,
		messagingHealth,
		consumerHealth,
	)
}

// NewMessagingHealthInfrastructure constructs the configured messaging health
// checker.
func (c *Config) NewMessagingHealthInfrastructure() (messaging.Health, error) {
	return c.Infrastructure.Messaging.newHealthInfrastructure()
}

// NewTraceInfrastructure constructs the configured tracing provider.
func (c *Config) NewTraceInfrastructure() (trace.Provider, error) {
	return c.Infrastructure.Trace.newTraceInfrastructure()
}

// NewSystemClockInfrastructure constructs the configured clock adapter.
func (c *Config) NewSystemClockInfrastructure() (clock.Clock, error) {
	return c.Infrastructure.Clock.newSystemClockInfrastructure()
}

// NewLoggerInfrastructure constructs the configured logger adapter.
func (c *Config) NewLoggerInfrastructure() (corelog.Logger, error) {
	return c.Infrastructure.Logger.newLoggerInfrastructure()
}

// NewErrorReporterInfrastructure constructs the configured unexpected-error reporter.
func (c *Config) NewErrorReporterInfrastructure() (coreerrorreporting.Reporter, error) {
	return c.Infrastructure.ErrorReporting.newErrorReporterInfrastructure(c.Runtime.Environment)
}

// NewMetricsInfrastructure constructs the configured metrics adapter.
// The selected provider must implement business metrics and the bounded
// outbound-HTTP operational metric contract used during bootstrap.
//
// The returned http.Handler serves Prometheus text exposition when the
// prometheus provider is selected. Bootstrap mounts it on a dedicated metrics
// HTTP server, not on the application REST engine.
func (c *Config) NewMetricsInfrastructure() (coremetrics.RuntimeMetrics, http.Handler, error) {
	return c.Infrastructure.Metrics.newMetricsInfrastructure()
}

// NewHTTPClientInfrastructure constructs the shared outbound HTTP/HTTPS client.
// External adapters should receive this client through bootstrap instead of
// constructing net/http.Client instances themselves.
func (c *Config) NewHTTPClientInfrastructure(
	clk clock.Clock,
	logger corelog.Logger,
	metrics coremetrics.OutboundHTTPMetrics,
) (corehttpclient.Client, error) {
	return c.Infrastructure.HTTPClient.newHTTPClientInfrastructure(clk, logger, metrics)
}

// MetricsServerEnabled reports whether the dedicated metrics scrape server should start.
func (c *Config) MetricsServerEnabled() bool {
	return c.Infrastructure.Metrics.ServerEnabled()
}

// GetMetricsListeningAddress returns the dedicated metrics server listen address.
func (c *Config) GetMetricsListeningAddress() string {
	return c.Infrastructure.Metrics.Prometheus.getListeningAddress()
}

// GetMetricsPath returns the validated Prometheus scrape path.
func (c *Config) GetMetricsPath() (string, error) {
	return c.Infrastructure.Metrics.Prometheus.normalizedPath()
}

// NewCacheInfrastructure constructs the configured cache adapter.
func (c *Config) NewCacheInfrastructure(ctx context.Context) (cache.Cache, error) {
	return c.Infrastructure.Cache.newCacheInfrastructure(ctx)
}

// NewObjectStorageInfrastructure constructs the configured object-storage adapter.
func (c *Config) NewObjectStorageInfrastructure() (coreobjectstorage.Store, error) {
	return c.Infrastructure.ObjectStorage.newObjectStorageInfrastructure()
}

// NewMessagingPublisherInfrastructure constructs the configured publisher adapter.
func (c *Config) NewMessagingPublisherInfrastructure() (messaging.Publisher, error) {
	return c.Infrastructure.Messaging.newPublisherInfrastructure()
}

// NewMessagingSubscriberInfrastructure constructs the configured subscriber adapter.
func (c *Config) NewMessagingSubscriberInfrastructure(
	logger corelog.Logger,
	publisher messaging.Publisher,
	consumerHealth corehealth.ConsumerStateWriter,
) (messaging.Subscriber, error) {
	return c.Infrastructure.Messaging.newSubscriberInfrastructureWithConsumer(logger, publisher, consumerHealth)
}

// NewTaskInfrastructure validates and constructs the configured task runtime adapter.
//
// Task validation is repeated here intentionally as a fail-closed initialization
// guard. Config.Load already validates the full document, but callers may build a
// Config programmatically. In particular, a noop task runtime must never be
// initialized in production.
func (c *Config) NewTaskInfrastructure(
	logger corelog.Logger,
	reporter coreerrorreporting.Reporter,
) (task.Runtime, error) {
	const op = "config.Config.NewTaskInfrastructure"

	env, err := c.Runtime.ParsedEnvironment()
	if err != nil {
		return nil, fmt.Errorf("%s: parse runtime environment for task infrastructure: %w", op, err)
	}

	if err = c.Infrastructure.Task.validate(env); err != nil {
		return nil, fmt.Errorf("%s: validate task infrastructure: %w", op, err)
	}

	return c.Infrastructure.Task.newTaskInfrastructure(logger, reporter)
}

// GetTaskDefinitions returns the startup-known task definitions from config.
func (c *Config) GetTaskDefinitions() ([]TaskDefinitionEntry, error) {
	return c.Infrastructure.Task.getTaskDefinitions()
}

// NewCircuitBreakerInfrastructure constructs the configured circuit breaker adapter.
func (c *Config) NewCircuitBreakerInfrastructure() (circuitbreaker.CircuitBreaker, error) {
	return c.Infrastructure.CircuitBreaker.newCircuitBreakerInfrastructure()
}

// NewExternalBotMessagingManager constructs all enabled bot messaging integrations.
func (c *Config) NewExternalBotMessagingManager(
	httpClient corehttpclient.Client,
	logger corelog.Logger,
) (*adapterbotmessage.Manager, error) {
	return c.External.BotMessaging.newManager(httpClient, logger)
}

// NewExternalCreditAssessmentClient constructs the configured external credit assessment client.
func (c *Config) NewExternalCreditAssessmentClient(
	breaker circuitbreaker.CircuitBreaker,
	clk clock.Clock,
) (extcreditassessment.Client, error) {
	return c.External.CreditAssessment.newExternalCreditAssessmentClient(breaker, clk)
}

// PaymentResultsTopic returns the configured Financial Core payment results topic name.
func (c *Config) PaymentResultsTopic() string {
	return c.Infrastructure.Messaging.Kafka.Topics.PaymentResults
}

// AuditEventsTopic returns the configured Kafka destination for BNPL audit events.
func (c *Config) AuditEventsTopic() string {
	return c.Infrastructure.Messaging.Kafka.Topics.AuditEvents
}

// PaymentApplicationResultsTopic returns the configured Kafka destination for BNPL
// payment-application acknowledgement outbox messages.
func (c *Config) PaymentApplicationResultsTopic() string {
	return c.Infrastructure.Messaging.Kafka.Topics.PaymentApplicationResults
}

// InstallmentPaymentRequestsTopic returns the configured Kafka destination for BNPL
// installment payment request outbox messages.
func (c *Config) InstallmentPaymentRequestsTopic() string {
	return c.Infrastructure.Messaging.Kafka.Topics.InstallmentPaymentRequests
}

// InstallmentPaymentSessionsTopic returns the configured Kafka topic for Financial Core
// installment payment-session-created events.
func (c *Config) InstallmentPaymentSessionsTopic() string {
	return c.Infrastructure.Messaging.Kafka.Topics.InstallmentPaymentSessions
}

// InstallmentPaymentReconciliationRequestsTopic returns the configured Kafka destination
// for BNPL installment payment reconciliation request outbox messages.
func (c *Config) InstallmentPaymentReconciliationRequestsTopic() string {
	return c.Infrastructure.Messaging.Kafka.Topics.InstallmentPaymentReconciliationRequests
}

// InstallmentPaymentReconciliationResultsTopic returns the configured Kafka topic for
// Financial Core installment payment reconciliation result events.
func (c *Config) InstallmentPaymentReconciliationResultsTopic() string {
	return c.Infrastructure.Messaging.Kafka.Topics.InstallmentPaymentReconciliationResults
}

// GetInstallmentPaymentReconciliationAcknowledgementsTopic returns the configured Kafka
// destination for BNPL reconciliation acknowledgement outbox messages.
func (c *Config) GetInstallmentPaymentReconciliationAcknowledgementsTopic() string {
	return c.Infrastructure.Messaging.Kafka.Topics.InstallmentPaymentReconciliationAcknowledgements
}

// NewExternalCheckoutPaymentClient constructs the configured Financial Core checkout payment client.
func (c *Config) NewExternalCheckoutPaymentClient(
	breaker circuitbreaker.CircuitBreaker,
) (extcheckoutpayment.Client, io.Closer, error) {
	return c.External.FinancialCore.newExternalCheckoutPaymentClient(breaker)
}

// NewExternalInstallmentPaymentClient constructs the configured Financial Core installment payment client.
func (c *Config) NewExternalInstallmentPaymentClient(
	breaker circuitbreaker.CircuitBreaker,
	clk clock.Clock,
) (extinstallmentpayment.Client, error) {
	return c.External.FinancialCore.newExternalInstallmentPaymentClient(breaker, clk)
}

// NewExternalContractCancellationClient constructs the configured Financial Core
// contract cancellation client.
func (c *Config) NewExternalContractCancellationClient() (
	extcontractcancellation.Client,
	error,
) {
	return c.External.FinancialCore.newExternalContractCancellationClient()
}

// NewExternalWebhookRoutingPublisher constructs the webhook destination router.
//
// webhook://merchant/<id> rows are handled by the configured webhook provider;
// every other destination keeps using fallback. The optional closer owns HTTP
// deliverer resources only — never the messaging fallback publisher.
func (c *Config) NewExternalWebhookRoutingPublisher(
	pool repo.Pool,
	checkoutSession repo.CheckoutSession,
	fallback messaging.Publisher,
	clk clock.Clock,
	logger corelog.Logger,
	businessMetrics coremetrics.BusinessMetrics,
	httpClient corehttpclient.Client,
) (messaging.Publisher, io.Closer, error) {
	const op = "config.Config.NewExternalWebhookRoutingPublisher"

	env, err := c.Runtime.ParsedEnvironment()
	if err != nil {
		return nil, nil, fmt.Errorf("%s: parse runtime environment: %w", op, err)
	}

	return c.External.WebhookDispatcher.newRoutingPublisher(
		c.Usecases.CheckoutSession,
		env,
		pool,
		checkoutSession,
		fallback,
		clk,
		logger,
		businessMetrics,
		httpClient,
	)
}

// String returns a JSON representation of c Config struct with secrets redacted.
func (c *Config) String() string {
	marshaledConfig, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return ""
	}

	return string(marshaledConfig)
}
