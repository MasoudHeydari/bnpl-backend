package paymentattemptreconciliationuc

import (
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/clock"
	"github.com/parstis/bnpl-backend/pkg/core/identifier"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// UseCase schedules payment attempt reconciliation work through the transactional outbox.
type UseCase struct {
	pool                              repo.Pool
	paymentAttempt                    repo.PaymentAttempt
	contract                          repo.Contract
	outbox                            repo.Outbox
	idGenerator                       identifier.Generator
	reconciliationRequestsDestination string
	auditDestination                  string
	batchSize                         int
	createdTimeout                    time.Duration
	submittedExpiryGrace              time.Duration
	retryBaseDelay                    time.Duration
	retryMaxDelay                     time.Duration
	maxAttempts                       int
	clock                             clock.Clock
}

// New constructs a payment attempt reconciliation use case with transactional Audit emission.
func New(
	pool repo.Pool,
	paymentAttempt repo.PaymentAttempt,
	contractRepo repo.Contract,
	outbox repo.Outbox,
	idGenerator identifier.Generator,
	reconciliationRequestsDestination string,
	auditDestination string,
	batchSize int,
	createdTimeout time.Duration,
	submittedExpiryGrace time.Duration,
	retryBaseDelay time.Duration,
	retryMaxDelay time.Duration,
	maxAttempts int,
	clk clock.Clock,
) (*UseCase, error) {
	return newUseCase(
		pool,
		paymentAttempt,
		contractRepo,
		outbox,
		idGenerator,
		reconciliationRequestsDestination,
		auditDestination,
		batchSize,
		createdTimeout,
		submittedExpiryGrace,
		retryBaseDelay,
		retryMaxDelay,
		maxAttempts,
		clk,
	)
}

func newUseCase(
	pool repo.Pool,
	paymentAttempt repo.PaymentAttempt,
	contractRepo repo.Contract,
	outbox repo.Outbox,
	idGenerator identifier.Generator,
	reconciliationRequestsDestination, auditDestination string,
	batchSize int,
	createdTimeout, submittedExpiryGrace, retryBaseDelay, retryMaxDelay time.Duration,
	maxAttempts int,
	clk clock.Clock,
) (*UseCase, error) {
	const op = "paymentattemptreconciliationuc.newUseCase"

	if pool == nil || paymentAttempt == nil || contractRepo == nil || outbox == nil || clk == nil {
		return nil, fmt.Errorf("%s: payment attempt reconciliation dependencies are required", op)
	}

	if idGenerator == nil {
		return nil, fmt.Errorf("%s: id generator is required", op)
	}

	if reconciliationRequestsDestination == "" {
		return nil, fmt.Errorf("%s: reconciliation requests destination is required", op)
	}

	if auditDestination == "" {
		return nil, fmt.Errorf("%s: audit destination is required", op)
	}

	if batchSize <= 0 {
		return nil, fmt.Errorf("%s: batch size must be positive", op)
	}

	if createdTimeout <= 0 {
		return nil, fmt.Errorf("%s: created timeout must be positive", op)
	}

	if submittedExpiryGrace < 0 {
		return nil, fmt.Errorf("%s: submitted expiry grace must not be negative", op)
	}

	if retryBaseDelay <= 0 {
		return nil, fmt.Errorf("%s: retry base delay must be positive", op)
	}

	if retryMaxDelay < retryBaseDelay {
		return nil, fmt.Errorf("%s: retry max delay must be greater than or equal to retry base delay", op)
	}

	if maxAttempts <= 0 {
		return nil, fmt.Errorf("%s: max reconciliation attempts must be positive", op)
	}

	return &UseCase{
		pool:                              pool,
		paymentAttempt:                    paymentAttempt,
		contract:                          contractRepo,
		outbox:                            outbox,
		idGenerator:                       idGenerator,
		reconciliationRequestsDestination: reconciliationRequestsDestination,
		auditDestination:                  auditDestination,
		batchSize:                         batchSize,
		createdTimeout:                    createdTimeout,
		submittedExpiryGrace:              submittedExpiryGrace,
		retryBaseDelay:                    retryBaseDelay,
		retryMaxDelay:                     retryMaxDelay,
		maxAttempts:                       maxAttempts,
		clock:                             clk,
	}, nil
}
