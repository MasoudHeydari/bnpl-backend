package outboxpublisheruc

import (
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
}

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
) *UseCase {
	if retryBaseDelay == 0 {
		retryBaseDelay = time.Second
	}

	if retryMaxDelay == 0 {
		retryMaxDelay = time.Minute
	}

	return &UseCase{
		pool:           pool,
		outbox:         outbox,
		publisher:      publisher,
		clock:          clock,
		logger:         logger,
		WorkerID:       workerID,
		BatchSize:      batchSize,
		RetryBaseDelay: retryBaseDelay,
		RetryMaxDelay:  retryMaxDelay,
	}
}
