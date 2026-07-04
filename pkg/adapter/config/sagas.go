package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/clock"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/saga"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/contractuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/sagaexecutoruc"
)

// Default Saga executor and domain-saga settings used when YAML/env values are unset.
const (
	DefaultSagaExecutorWorkerID             = "saga-executor"
	DefaultSagaExecutorBatchSize            = 10
	DefaultSagaExecutorLeaseDuration        = 2 * time.Minute
	DefaultSagaExecutorPersistenceTimeout   = 5 * time.Second
	DefaultCheckoutConfirmationSagaRetryBase = time.Second
	DefaultCheckoutConfirmationSagaRetryMax  = time.Minute
	DefaultCheckoutConfirmationSagaMaxRetry  = 8
	DefaultContractCancellationSagaRetryBase = time.Second
	DefaultContractCancellationSagaRetryMax  = time.Minute
)

// Sagas centralizes durable Saga executor and domain-saga behavioral settings.
// Task scheduling for the saga_executor job remains under infrastructure.task.
type Sagas struct {
	// Executor configures the generic durable Saga executor.
	Executor SagaExecutor `yaml:"executor"`

	// CheckoutConfirmation configures checkout-confirmation saga retry policy.
	CheckoutConfirmation CheckoutConfirmationSaga `yaml:"checkout_confirmation"`

	// ContractCancellation configures contract-cancellation saga retry policy.
	ContractCancellation ContractCancellationSaga `yaml:"contract_cancellation"`
}

// ApplyDefaults fills unset Saga settings under usecases.sagas.
func (s *Sagas) ApplyDefaults() {
	s.Executor.ApplyDefaults()
	s.CheckoutConfirmation.ApplyDefaults()
	s.ContractCancellation.ApplyDefaults()
}

func (s *Sagas) validate() error {
	if err := s.Executor.validate(); err != nil {
		return err
	}

	if err := s.CheckoutConfirmation.validate(); err != nil {
		return err
	}

	return s.ContractCancellation.validate()
}

// SagaExecutor configures the generic durable Saga executor.
type SagaExecutor struct {
	// WorkerID is the identity used when claiming saga rows for execution.
	WorkerID string `yaml:"worker_id"`

	// BatchSize configures how many saga rows are claimed per execution cycle.
	BatchSize int `yaml:"batch_size"`

	// LeaseDuration configures how long a claimed saga row remains leased.
	LeaseDuration time.Duration `yaml:"lease_duration"`

	// PersistenceTimeout bounds detached durable persistence after an external
	// step finishes so a canceled HTTP request cannot leave the write unbounded.
	PersistenceTimeout time.Duration `yaml:"persistence_timeout"`
}

// ApplyDefaults fills unset Saga executor settings.
func (s *SagaExecutor) ApplyDefaults() {
	if strings.TrimSpace(s.WorkerID) == "" {
		s.WorkerID = DefaultSagaExecutorWorkerID
	}

	if s.BatchSize == 0 {
		s.BatchSize = DefaultSagaExecutorBatchSize
	}

	if s.LeaseDuration == 0 {
		s.LeaseDuration = DefaultSagaExecutorLeaseDuration
	}

	if s.PersistenceTimeout == 0 {
		s.PersistenceTimeout = DefaultSagaExecutorPersistenceTimeout
	}
}

func (s *SagaExecutor) validate() error {
	const op = "config.SagaExecutor.validate"

	if strings.TrimSpace(s.WorkerID) == "" {
		return fmt.Errorf("%s: invalid usecases.sagas.executor.worker_id: value is required", op)
	}

	if s.BatchSize <= 0 {
		return fmt.Errorf("%s: invalid usecases.sagas.executor.batch_size: must be positive", op)
	}

	if s.LeaseDuration <= 0 {
		return fmt.Errorf("%s: invalid usecases.sagas.executor.lease_duration: must be positive", op)
	}

	if s.PersistenceTimeout <= 0 {
		return fmt.Errorf("%s: invalid usecases.sagas.executor.persistence_timeout: must be positive", op)
	}

	return nil
}

func (s SagaExecutor) newUseCase(
	pool repo.Pool,
	sagaRepo repo.Saga,
	definitions saga.DefinitionResolver,
	clk clock.Clock,
) *sagaexecutoruc.UseCase {
	s.ApplyDefaults()
	return sagaexecutoruc.New(
		pool,
		sagaRepo,
		definitions,
		clk,
		s.WorkerID,
		s.BatchSize,
		s.LeaseDuration,
		s.PersistenceTimeout,
	)
}

// CheckoutConfirmationSaga configures business-specific retry timing and budget.
type CheckoutConfirmationSaga struct {
	RetryBaseDelay time.Duration `yaml:"retry_base_delay"`
	RetryMaxDelay  time.Duration `yaml:"retry_max_delay"`

	// MaxRetryAttempts is the number of additional retries after the initial
	// step execution. Default 8 means 1 initial + 8 retries = 9 total claims
	// of one retryable step before budget exhaustion.
	MaxRetryAttempts int `yaml:"max_retry_attempts"`
}

func (d *CheckoutConfirmationSaga) ApplyDefaults() {
	if d.RetryBaseDelay == 0 {
		d.RetryBaseDelay = DefaultCheckoutConfirmationSagaRetryBase
	}

	if d.RetryMaxDelay == 0 {
		d.RetryMaxDelay = DefaultCheckoutConfirmationSagaRetryMax
	}

	if d.MaxRetryAttempts == 0 {
		d.MaxRetryAttempts = DefaultCheckoutConfirmationSagaMaxRetry
	}
}

func (d *CheckoutConfirmationSaga) validate() error {
	const op = "config.CheckoutConfirmationSaga.validate"

	if d.RetryBaseDelay <= 0 {
		return fmt.Errorf("%s: invalid usecases.sagas.checkout_confirmation.retry_base_delay: must be positive", op)
	}

	if d.RetryMaxDelay < d.RetryBaseDelay {
		return fmt.Errorf("%s: invalid usecases.sagas.checkout_confirmation.retry_max_delay: must be greater than or equal to retry_base_delay", op)
	}

	if d.MaxRetryAttempts <= 0 {
		return fmt.Errorf("%s: invalid usecases.sagas.checkout_confirmation.max_retry_attempts: must be positive", op)
	}

	return nil
}

// ContractCancellationSaga configures business-specific retry timing. Generic
// claim/lease behavior is owned by usecases.sagas.executor.
type ContractCancellationSaga struct {
	// RetryBaseDelay configures the initial delay before retrying a cancellation saga step.
	RetryBaseDelay time.Duration `yaml:"retry_base_delay"`

	// RetryMaxDelay configures the maximum delay between cancellation saga step retries.
	RetryMaxDelay time.Duration `yaml:"retry_max_delay"`
}

func (d *ContractCancellationSaga) ApplyDefaults() {
	if d.RetryBaseDelay == 0 {
		d.RetryBaseDelay = DefaultContractCancellationSagaRetryBase
	}

	if d.RetryMaxDelay == 0 {
		d.RetryMaxDelay = DefaultContractCancellationSagaRetryMax
	}
}

func (d *ContractCancellationSaga) ToUseCaseSettings() contractuc.CancellationSagaSettings {
	return contractuc.CancellationSagaSettings{
		RetryBaseDelay: d.RetryBaseDelay,
		RetryMaxDelay:  d.RetryMaxDelay,
	}
}

func (d *ContractCancellationSaga) validate() error {
	const op = "config.ContractCancellationSaga.validate"

	if d.RetryBaseDelay <= 0 {
		return fmt.Errorf("%s: invalid usecases.sagas.contract_cancellation.retry_base_delay: must be positive", op)
	}

	if d.RetryMaxDelay < d.RetryBaseDelay {
		return fmt.Errorf("%s: invalid usecases.sagas.contract_cancellation.retry_max_delay: must be greater than or equal to retry_base_delay", op)
	}

	return nil
}
