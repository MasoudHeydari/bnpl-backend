package paymentattemptexpirationuc

import (
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/clock"
	"github.com/parstis/bnpl-backend/pkg/core/identifier"
	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// UseCase expires eligible installment repayment PaymentAttempts locally.
type UseCase struct {
	pool             repo.Pool
	paymentAttempt   repo.PaymentAttempt
	outbox           repo.Outbox
	idGenerator      identifier.Generator
	auditDestination string
	batchSize        int
	safetyAfterLease time.Duration
	clock            clock.Clock
	logger           log.Logger
}

// New constructs the local session-creation expiration use case.
func New(
	pool repo.Pool,
	paymentAttempt repo.PaymentAttempt,
	outbox repo.Outbox,
	idGenerator identifier.Generator,
	auditDestination string,
	batchSize int,
	safetyAfterLease time.Duration,
	clk clock.Clock,
	logger log.Logger,
) (*UseCase, error) {
	const op = "paymentattemptexpirationuc.New"

	if pool == nil {
		return nil, fmt.Errorf("%s: pool is required", op)
	}

	if paymentAttempt == nil {
		return nil, fmt.Errorf("%s: payment attempt repository is required", op)
	}

	if outbox == nil {
		return nil, fmt.Errorf("%s: outbox is required", op)
	}

	if idGenerator == nil {
		return nil, fmt.Errorf("%s: id generator is required", op)
	}

	if auditDestination == "" {
		return nil, fmt.Errorf("%s: audit destination is required", op)
	}

	if batchSize <= 0 {
		return nil, fmt.Errorf("%s: batch size must be positive", op)
	}

	if safetyAfterLease < 0 {
		return nil, fmt.Errorf("%s: safety after lease must not be negative", op)
	}

	if safetyAfterLease == 0 {
		safetyAfterLease = model.FinancialSessionCreateExpirationSafetyInterval
	}

	if clk == nil {
		return nil, fmt.Errorf("%s: clock is required", op)
	}

	if logger == nil {
		return nil, fmt.Errorf("%s: logger is required", op)
	}

	return &UseCase{
		pool:             pool,
		paymentAttempt:   paymentAttempt,
		outbox:           outbox,
		idGenerator:      idGenerator,
		auditDestination: auditDestination,
		batchSize:        batchSize,
		safetyAfterLease: safetyAfterLease,
		clock:            clk,
		logger:           logger,
	}, nil
}
