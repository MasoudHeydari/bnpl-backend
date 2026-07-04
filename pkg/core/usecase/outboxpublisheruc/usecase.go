package outboxpublisheruc

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/clock"
	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/messaging"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// UseCase publishes pending outbox messages and updates their delivery state.
type UseCase struct {
	pool      repo.Pool
	outbox    repo.Outbox
	publisher messaging.Publisher
	clock     clock.Clock
	logger    log.Logger
	// WorkerID uniquely identifies the worker instance claiming messages.
	WorkerID string

	// BatchSize is the maximum number of messages claimed in a single run.
	BatchSize int

	// RetryBaseDelay is the initial retry delay.
	RetryBaseDelay time.Duration

	// RetryMaxDelay is the upper bound for exponential backoff.
	RetryMaxDelay time.Duration

	// ProcessingLeaseTimeout bounds how long a claimed outbox row may remain processing.
	ProcessingLeaseTimeout time.Duration
}

// Outbox publisher retry and lease configuration errors.
var (
	ErrInvalidRetryBaseDelay         = errors.New("invalid outbox publisher retry base delay")
	ErrInvalidRetryMaxDelay          = errors.New("invalid outbox publisher retry max delay")
	ErrInvalidProcessingLeaseTimeout = errors.New("invalid outbox publisher processing lease timeout")
)

// New constructs an outbox publishing use case.
func New(
	pool repo.Pool,
	outbox repo.Outbox,
	publisher messaging.Publisher,
	clock clock.Clock,
	logger log.Logger,
	workerID string,
	batchSize int,
	retryBaseDelay time.Duration,
	retryMaxDelay time.Duration,
	processingLeaseTimeout time.Duration,
) *UseCase {
	if retryBaseDelay == 0 {
		retryBaseDelay = time.Second
	}

	if retryMaxDelay == 0 {
		retryMaxDelay = time.Minute
	}

	if processingLeaseTimeout == 0 {
		processingLeaseTimeout = 5 * time.Minute
	}

	return &UseCase{
		pool:                   pool,
		outbox:                 outbox,
		publisher:              publisher,
		clock:                  clock,
		logger:                 logger,
		WorkerID:               workerID,
		BatchSize:              batchSize,
		RetryBaseDelay:         retryBaseDelay,
		RetryMaxDelay:          retryMaxDelay,
		ProcessingLeaseTimeout: processingLeaseTimeout,
	}
}

// Validate rejects incomplete dependencies and invalid runtime parameters.
func (uc *UseCase) Validate() error {
	const op = "outboxpublisheruc.UseCase.Validate"

	if uc == nil {
		return errors.New("outbox publisher use case is nil")
	}

	if uc.pool == nil || uc.outbox == nil || uc.publisher == nil || uc.clock == nil || uc.logger == nil {
		return errors.New("outbox publisher dependencies must not be nil")
	}

	if strings.TrimSpace(uc.WorkerID) == "" {
		return ErrMissingWorkerID
	}

	if uc.BatchSize <= 0 {
		return ErrInvalidBatchSize
	}

	if uc.RetryBaseDelay <= 0 {
		return ErrInvalidRetryBaseDelay
	}

	if uc.RetryMaxDelay < uc.RetryBaseDelay {
		return fmt.Errorf("%s: %w: max delay must be greater than or equal to base delay", op, ErrInvalidRetryMaxDelay)
	}

	if uc.ProcessingLeaseTimeout <= 0 {
		return ErrInvalidProcessingLeaseTimeout
	}

	return nil
}
