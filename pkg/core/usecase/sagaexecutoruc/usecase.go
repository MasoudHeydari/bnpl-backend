package sagaexecutoruc

import (
	"errors"
	"strings"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/clock"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/saga"
)

var (
	ErrInvalidDependencies = errors.New("invalid saga executor dependencies")
	ErrInvalidWorkerID     = errors.New("invalid saga executor worker id")
	ErrInvalidBatchSize    = errors.New("invalid saga executor batch size")
	ErrInvalidLease        = errors.New("invalid saga executor lease duration")
	ErrInvalidStepContract = errors.New("invalid saga step execution contract")
)

// UseCase executes durable Saga instances independently of any concrete
// persistence, scheduler, or transport implementation.
type UseCase struct {
	pool        repo.Pool
	sagas       repo.Saga
	definitions saga.DefinitionResolver
	clock       clock.Clock

	// WorkerID uniquely identifies the worker instance claiming sagas.
	WorkerID string
	// BatchSize is the maximum number of saga rows claimed per run.
	BatchSize int
	// LeaseDuration is how long a claimed saga lease remains valid.
	LeaseDuration time.Duration
}

// New constructs a Saga executor use case.
func New(
	pool repo.Pool,
	sagas repo.Saga,
	definitions saga.DefinitionResolver,
	clock clock.Clock,
	workerID string,
	batchSize int,
	leaseDuration time.Duration,
) *UseCase {
	return &UseCase{
		pool:          pool,
		sagas:         sagas,
		definitions:   definitions,
		clock:         clock,
		WorkerID:      workerID,
		BatchSize:     batchSize,
		LeaseDuration: leaseDuration,
	}
}

// Validate rejects incomplete dependencies and invalid runtime parameters.
func (uc *UseCase) Validate() error {
	if uc == nil || uc.pool == nil || uc.sagas == nil || uc.definitions == nil || uc.clock == nil {
		return ErrInvalidDependencies
	}

	if strings.TrimSpace(uc.WorkerID) == "" {
		return ErrInvalidWorkerID
	}

	if uc.BatchSize <= 0 {
		return ErrInvalidBatchSize
	}

	if uc.LeaseDuration <= 0 {
		return ErrInvalidLease
	}

	return nil
}
