package contractuc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/clock"
	"github.com/parstis/bnpl-backend/pkg/core/domaincapability"
	extcheckoutpayment "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
	extcontractcancellation "github.com/parstis/bnpl-backend/pkg/core/external/contractcancellation"
	"github.com/parstis/bnpl-backend/pkg/core/identifier"
	"github.com/parstis/bnpl-backend/pkg/core/log"
	coremetrics "github.com/parstis/bnpl-backend/pkg/core/metrics"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/saga"
)

// CancellationSagaSettings contains business-specific retry timing for the
// contract-cancellation-request Saga. Generic worker batch and lease settings
// belong to the Saga executor configuration.
type CancellationSagaSettings struct {
	// RetryBaseDelay is the initial retry delay.
	RetryBaseDelay time.Duration
	// RetryMaxDelay is the upper bound for exponential backoff.
	RetryMaxDelay time.Duration
}

// UseCase orchestrates checkout contract creation and activation.
type UseCase struct {
	pool                       repo.Pool
	customerProfile            repo.CustomerProfile
	merchantProfile            repo.MerchantProfile
	creditAccount              repo.CreditAccount
	decision                   repo.Decision
	contract                   repo.Contract
	checkoutDecisionAllocation repo.CheckoutDecisionAllocation
	checkoutPolicies           *checkout.Registry
	installment                repo.Installment
	installmentCharge          repo.InstallmentCharge
	paymentAttempt             repo.PaymentAttempt
	checkoutPaymentClient      extcheckoutpayment.Client
	contractCancellationClient extcontractcancellation.Client
	outbox                     repo.Outbox
	clock                      clock.Clock
	logger                     log.Logger
	auditDestination           string
	idGenerator                identifier.Generator
	businessMetrics            coremetrics.BusinessMetrics
	paymentReturnURL           string

	// jurisdictions gates creation of a first Contract for a Decision.
	// It must not be used to block servicing/replay of an existing Contract.
	jurisdictions domaincapability.OriginationJurisdictions

	// Cancellation Saga dependencies are required at construction. Keeping them
	// behind Core interfaces avoids coupling this use case to a concrete worker
	// or persistence implementation.
	cancellationSagaRepo       repo.Saga
	cancellationSagaExecutor   saga.Executor
	cancellationSagaDefinition saga.Definition

	// PaymentResultProcessingGracePeriod tolerates delayed Financial Core payment-result
	// delivery and BNPL processing after the effective payment deadline (0–60m).
	PaymentResultProcessingGracePeriod time.Duration

	// ExpiryBatchSize is the maximum number of payment_pending contracts expired per job run.
	ExpiryBatchSize int

	// CancellationSaga executes contract cancellation saga steps.
	CancellationSaga CancellationSagaSettings

	// BeforeTerminalCheckoutPaymentReplacement is an optional test barrier.
	// It runs after a terminal checkout attempt is observed and before the
	// replacement row is inserted, outside the database transaction.
	// Production leaves it nil.
	BeforeTerminalCheckoutPaymentReplacement func(context.Context) error
}

// New constructs the checkout contract use case with payment-attempt persistence,
// transactional audit outbox support, and required cancellation Saga wiring.
//
// cancellationSagaRepo, cancellationSagaExecutor, and cancellationSagaDefinition
// are all required. The definition must match the contract-cancellation Saga
// type/version.
func New(
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
	paymentResultProcessingGracePeriod time.Duration,
	expiryBatchSize int,
	cancellationSaga CancellationSagaSettings,
	cancellationSagaRepo repo.Saga,
	cancellationSagaExecutor saga.Executor,
	cancellationSagaDefinition saga.Definition,
	jurisdictions domaincapability.OriginationJurisdictions,
	logger log.Logger,
	businessMetrics coremetrics.BusinessMetrics,
) (*UseCase, error) {
	const op = "contractuc.New"

	if pool == nil {
		return nil, fmt.Errorf("%s: pool is required", op)
	}

	if customerProfile == nil {
		return nil, fmt.Errorf("%s: customer profile repository is required", op)
	}

	if merchantProfile == nil {
		return nil, fmt.Errorf("%s: merchant profile repository is required", op)
	}

	if creditAccount == nil {
		return nil, fmt.Errorf("%s: credit account repository is required", op)
	}

	if decision == nil {
		return nil, fmt.Errorf("%s: decision repository is required", op)
	}

	if contractRepo == nil {
		return nil, fmt.Errorf("%s: contract repository is required", op)
	}

	if checkoutDecisionAllocation == nil {
		return nil, fmt.Errorf("%s: checkout decision allocation repository is required", op)
	}

	if checkoutPolicies == nil {
		return nil, fmt.Errorf("%s: checkout policy registry is required", op)
	}

	if installment == nil {
		return nil, fmt.Errorf("%s: installment repository is required", op)
	}

	if installmentCharge == nil {
		return nil, fmt.Errorf("%s: installment charge repository is required", op)
	}

	if checkoutPaymentClient == nil {
		return nil, fmt.Errorf("%s: checkout payment client is required", op)
	}

	if contractCancellationClient == nil {
		return nil, fmt.Errorf("%s: contract cancellation client is required", op)
	}

	if paymentAttempt == nil {
		return nil, fmt.Errorf("%s: payment attempt repository is required", op)
	}

	if outbox == nil {
		return nil, fmt.Errorf("%s: contract audit outbox repository is required", op)
	}

	if auditDestination == "" {
		return nil, fmt.Errorf("%s: contract audit destination is required", op)
	}

	if idGenerator == nil {
		return nil, fmt.Errorf("%s: contract audit id generator is required", op)
	}

	if clk == nil {
		return nil, fmt.Errorf("%s: clock is required", op)
	}

	if strings.TrimSpace(paymentReturnURL) == "" {
		return nil, fmt.Errorf("%s: payment return url is required", op)
	}

	if expiryBatchSize <= 0 {
		return nil, fmt.Errorf("%s: contract expiry batch size must be positive", op)
	}

	if paymentResultProcessingGracePeriod < 0 {
		return nil, fmt.Errorf("%s: contract payment result processing grace period must not be negative", op)
	}

	if paymentResultProcessingGracePeriod > model.MaxPaymentResultProcessingGracePeriod {
		return nil, fmt.Errorf(
			"%s: contract payment result processing grace period must not exceed %s", op, model.MaxPaymentResultProcessingGracePeriod,
		)
	}

	if cancellationSaga.RetryBaseDelay <= 0 {
		return nil, fmt.Errorf("%s: contract cancellation saga retry base delay must be positive", op)
	}

	if cancellationSaga.RetryMaxDelay < cancellationSaga.RetryBaseDelay {
		return nil, fmt.Errorf("%s: contract cancellation saga retry max delay must be greater than or equal to retry base delay", op)
	}

	if cancellationSagaRepo == nil {
		return nil, fmt.Errorf("%s: contract cancellation saga repository is required", op)
	}

	if cancellationSagaExecutor == nil {
		return nil, fmt.Errorf("%s: contract cancellation saga executor is required", op)
	}

	if cancellationSagaDefinition == nil {
		return nil, fmt.Errorf("%s: contract cancellation saga definition is required", op)
	}

	if err := saga.ValidateDefinition(cancellationSagaDefinition); err != nil {
		return nil, fmt.Errorf("%s: validate contract cancellation saga definition: %w", op, err)
	}

	if cancellationSagaDefinition.Type() != contractCancellationSagaType ||
		cancellationSagaDefinition.Version() != contractCancellationSagaVersion {
		return nil, fmt.Errorf(
			"%s: unexpected contract cancellation saga definition %q version %d", op, cancellationSagaDefinition.Type(),
			cancellationSagaDefinition.Version(),
		)
	}

	if jurisdictions == nil {
		return nil, fmt.Errorf("%s: origination jurisdictions is required", op)
	}

	if logger == nil {
		return nil, fmt.Errorf("%s: logger is required", op)
	}

	if businessMetrics == nil {
		return nil, fmt.Errorf("%s: business metrics is required", op)
	}

	return &UseCase{
		pool:                               pool,
		customerProfile:                    customerProfile,
		merchantProfile:                    merchantProfile,
		creditAccount:                      creditAccount,
		decision:                           decision,
		contract:                           contractRepo,
		checkoutDecisionAllocation:         checkoutDecisionAllocation,
		checkoutPolicies:                   checkoutPolicies,
		installment:                        installment,
		installmentCharge:                  installmentCharge,
		paymentAttempt:                     paymentAttempt,
		checkoutPaymentClient:              checkoutPaymentClient,
		contractCancellationClient:         contractCancellationClient,
		outbox:                             outbox,
		clock:                              clk,
		logger:                             logger,
		auditDestination:                   auditDestination,
		idGenerator:                        idGenerator,
		paymentReturnURL:                   strings.TrimSpace(paymentReturnURL),
		jurisdictions:                      jurisdictions,
		cancellationSagaRepo:               cancellationSagaRepo,
		cancellationSagaExecutor:           cancellationSagaExecutor,
		cancellationSagaDefinition:         cancellationSagaDefinition,
		PaymentResultProcessingGracePeriod: paymentResultProcessingGracePeriod,
		ExpiryBatchSize:                    expiryBatchSize,
		CancellationSaga:                   cancellationSaga,
		businessMetrics:                    businessMetrics,
	}, nil
}
