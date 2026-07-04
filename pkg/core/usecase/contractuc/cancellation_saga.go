package contractuc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/clock"
	extcontractcancellation "github.com/parstis/bnpl-backend/pkg/core/external/contractcancellation"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/saga"
)

const (
	contractCancellationSagaType    model.SagaType              = "contract_cancellation_request"
	contractCancellationSagaVersion model.SagaDefinitionVersion = 1

	contractCancellationLocalStartedStep     model.SagaStep = "contract_cancellation_local_started"
	contractCancellationExternalStep         model.SagaStep = "contract_cancellation_external_request"
	contractCancellationExternalAcceptedStep model.SagaStep = "contract_cancellation_external_accepted"

	contractCancellationSagaSubjectType = "contract"
)

// CancellationSagaDefinition describes the durable contract-cancellation
// process executed by the generic Saga executor.
//
// The local cancellation start is committed before the external request. If the
// external request is definitively rejected, the local start is compensated by
// returning the contract from canceling to active. Retryable failures keep the
// external step runnable with a delayed NextAttemptAt.
type CancellationSagaDefinition struct {
	localStep    *cancellationLocalStartedStep
	externalStep *cancellationExternalRequestStep
	acceptedStep *cancellationExternalAcceptedStep
}

var _ saga.Definition = (*CancellationSagaDefinition)(nil)

// NewCancellationSagaDefinition constructs the contract-cancellation Saga
// definition and its business steps.
func NewCancellationSagaDefinition(
	pool repo.Pool,
	contractRepo repo.Contract,
	creditAccount repo.CreditAccount,
	client extcontractcancellation.Client,
	clk clock.Clock,
	retryBaseDelay time.Duration,
	retryMaxDelay time.Duration,
) (*CancellationSagaDefinition, error) {
	const op = "contractuc.NewCancellationSagaDefinition"

	if pool == nil {
		return nil, fmt.Errorf("%s: pool is required", op)
	}

	if contractRepo == nil {
		return nil, fmt.Errorf("%s: contract repository is required", op)
	}

	if creditAccount == nil {
		return nil, fmt.Errorf("%s: credit account repository is required", op)
	}

	if client == nil {
		return nil, fmt.Errorf("%s: contract cancellation client is required", op)
	}

	if clk == nil {
		return nil, fmt.Errorf("%s: clock is required", op)
	}

	if retryBaseDelay <= 0 {
		return nil, fmt.Errorf("%s: contract cancellation saga retry base delay must be positive", op)
	}

	if retryMaxDelay < retryBaseDelay {
		return nil, fmt.Errorf("%s: contract cancellation saga retry max delay must be greater than or equal to retry base delay", op)
	}

	local := &cancellationLocalStartedStep{
		contract: contractRepo,
		clock:    clk,
	}
	external := &cancellationExternalRequestStep{
		pool:           pool,
		contract:       contractRepo,
		client:         client,
		clock:          clk,
		retryBaseDelay: retryBaseDelay,
		retryMaxDelay:  retryMaxDelay,
	}
	accepted := &cancellationExternalAcceptedStep{
		contract:      contractRepo,
		creditAccount: creditAccount,
		clock:         clk,
	}

	return &CancellationSagaDefinition{
		localStep:    local,
		externalStep: external,
		acceptedStep: accepted,
	}, nil
}

// Type returns the stable Saga type.
func (d *CancellationSagaDefinition) Type() model.SagaType {
	return contractCancellationSagaType
}

// Version returns the immutable definition version.
func (d *CancellationSagaDefinition) Version() model.SagaDefinitionVersion {
	return contractCancellationSagaVersion
}

// InitialStep returns the durable local-start milestone.
func (d *CancellationSagaDefinition) InitialStep() model.SagaStep {
	return contractCancellationLocalStartedStep
}

// IsForwardTerminalStep reports that the authoritative synchronous Financial Core
// result has been durably applied to the local contract.
func (d *CancellationSagaDefinition) IsForwardTerminalStep(step model.SagaStep) bool {
	return step == contractCancellationExternalAcceptedStep
}

// Step resolves a stable step identifier to its implementation.
func (d *CancellationSagaDefinition) Step(name model.SagaStep) (saga.Step, bool) {
	switch name {
	case contractCancellationLocalStartedStep:
		return d.localStep, true

	case contractCancellationExternalStep:
		return d.externalStep, true

	case contractCancellationExternalAcceptedStep:
		return d.acceptedStep, true

	default:
		return nil, false
	}
}

// CompensationFor declares how completed forward work is compensated.
func (d *CancellationSagaDefinition) CompensationFor(
	completedStep model.SagaStep,
) (saga.Compensation, bool) {
	switch completedStep {
	case contractCancellationLocalStartedStep:
		return saga.Compensation{Mode: saga.CompensationModeDirect}, true

	case contractCancellationExternalStep, contractCancellationExternalAcceptedStep:
		return saga.Compensation{Mode: saga.CompensationModeNone}, true

	default:
		return saga.Compensation{}, false
	}
}

// CanTransition defines the forward graph. Direct compensation reuses the
// completed local step and therefore does not require a compensating-step edge.
func (d *CancellationSagaDefinition) CanTransition(
	phase model.SagaPhase,
	from model.SagaStep,
	to model.SagaStep,
) bool {
	if phase != model.ForwardSagaPhase {
		return false
	}

	switch from {
	case contractCancellationLocalStartedStep:
		return to == contractCancellationExternalStep

	case contractCancellationExternalStep:
		return to == contractCancellationExternalAcceptedStep

	default:
		return false
	}
}

type cancellationLocalStartedStep struct {
	profiles repo.CustomerProfile
	contract repo.Contract
	clock    clock.Clock
}

var (
	_ saga.TransactionalStep              = (*cancellationLocalStartedStep)(nil)
	_ saga.TransactionalCompensatableStep = (*cancellationLocalStartedStep)(nil)
)

// Name returns the saga step name.
func (s *cancellationLocalStartedStep) Name() model.SagaStep {
	return contractCancellationLocalStartedStep
}

// ExecuteTx validates the local cancellation milestone. The command that starts
// the Saga performs active -> canceling and Saga creation in the same local
// transaction, then records this step as completed before the transaction
// commits. ExecuteTx remains available so the definition is self-contained and
// the step can be validated consistently by the generic executor.
func (s *cancellationLocalStartedStep) ExecuteTx(
	ctx context.Context,
	tx repo.Tx,
	instance model.SagaInstance,
) (saga.StepResult, error) {
	const op = "contractuc.cancellationLocalStartedStep.ExecuteTx"

	contractID, err := cancellationSagaContractID(instance)
	if err != nil {
		return saga.StepResult{}, err
	}

	contractQ := s.contract.Tx(tx)
	if err := lockCancellationCustomer(ctx, tx, s.profiles, contractQ, contractID); err != nil {
		return saga.StepResult{}, err
	}
	contractRow, err := contractQ.LockByID(ctx, contractID)
	if err != nil {
		return saga.StepResult{}, err
	}

	if contractRow.Status != model.CancelingContractStatus {
		return saga.StepResult{}, fmt.Errorf(
			"%s: contract cancellation saga expected canceling contract %s, got %s", op, contractID,
			contractRow.Status,
		)
	}

	next := contractCancellationExternalStep
	return saga.StepResult{
		Outcome:  saga.StepOutcomeCompleted,
		NextStep: &next,
	}, nil
}

// CompensateTx reverses only the local cancellation-start state after the
// external request has been definitively rejected. It must never be selected
// for timeout or ambiguous outcomes.
func (s *cancellationLocalStartedStep) CompensateTx(
	ctx context.Context,
	tx repo.Tx,
	instance model.SagaInstance,
) (saga.StepResult, error) {
	contractID, err := cancellationSagaContractID(instance)
	if err != nil {
		return saga.StepResult{}, err
	}

	contractQ := s.contract.Tx(tx)
	if err := lockCancellationCustomer(ctx, tx, s.profiles, contractQ, contractID); err != nil {
		return saga.StepResult{}, err
	}
	contractRow, err := contractQ.LockByID(ctx, contractID)
	if err != nil {
		return saga.StepResult{}, err
	}

	switch contractRow.Status {
	case model.ActiveContractStatus:
		// Defensive idempotency: if a previous attempt already restored the local
		// state in the same logical compensation path, no further write is needed.
		return saga.StepResult{Outcome: saga.StepOutcomeCompleted}, nil

	case model.CancelingContractStatus:
		if contractRow.CancellationFinancialCoreAcceptedAt != nil {
			reason := fmt.Sprintf(
				"contract %s external cancellation was already accepted before compensation",
				contractID,
			)
			return saga.StepResult{
				Outcome: saga.StepOutcomeReconcile,
				Reason:  &reason,
			}, nil
		}

		if err := contractRow.Status.ValidateTransitionTo(model.ActiveContractStatus); err != nil {
			return saga.StepResult{}, err
		}

		if _, err := contractQ.RestoreActiveAfterCancellationRejected(
			ctx,
			contractID,
			s.clock.Now().UTC(),
		); err != nil {
			return saga.StepResult{}, err
		}

		return saga.StepResult{Outcome: saga.StepOutcomeCompleted}, nil

	default:
		reason := fmt.Sprintf("contract %s moved to %s while cancellation compensation was pending", contractID, contractRow.Status)
		return saga.StepResult{
			Outcome: saga.StepOutcomeReconcile,
			Reason:  &reason,
		}, nil
	}
}

type cancellationExternalRequestStep struct {
	pool           repo.Pool
	contract       repo.Contract
	client         extcontractcancellation.Client
	clock          clock.Clock
	retryBaseDelay time.Duration
	retryMaxDelay  time.Duration
}

var _ saga.ExternalStep = (*cancellationExternalRequestStep)(nil)

// Name returns the saga step name.
func (s *cancellationExternalRequestStep) Name() model.SagaStep {
	return contractCancellationExternalStep
}

// Execute performs the authoritative synchronous cancellation request. The call
// intentionally runs outside a local database transaction. A successful result
// means Financial Core has completed the requested cancellation operation.
func (s *cancellationExternalRequestStep) Execute(
	ctx context.Context,
	instance model.SagaInstance,
) (saga.StepResult, error) {
	const op = "contractuc.cancellationExternalRequestStep.Execute"

	contractRow, err := s.loadContract(ctx, instance)
	if err != nil {
		return saga.StepResult{}, err
	}

	if contractRow.Status != model.CancelingContractStatus {
		reason := fmt.Sprintf("contract %s is %s while external cancellation step is runnable", contractRow.ID, contractRow.Status)
		return saga.StepResult{Outcome: saga.StepOutcomeReconcile, Reason: &reason}, nil
	}

	if contractRow.CancellationFinancialCoreIdempotencyKey == nil ||
		contractRow.CancellationTenantID == nil ||
		contractRow.CancellationContextID == nil ||
		contractRow.CancellationReason == nil {
		return saga.StepResult{}, fmt.Errorf("%s: canceling contract %s has incomplete cancellation metadata", op, contractRow.ID)
	}

	// The concrete Financial Core command is intentionally represented by the
	// transport-neutral port. When the Financial Core API is finalized, this
	// request is expected to ask that service to perform its authoritative bundle:
	//
	//   1. validate that the cancellation can be accepted against current
	//      financial state;
	//   2. perform the required financial reversal/refund/settlement operations;
	//   3. durably complete the authoritative cancellation operation using the
	//      same stable idempotency identity.
	//
	// The exact monetary operations belong to Financial Core and are therefore
	// not implemented or inferred in this Saga step.
	result, err := s.client.RequestCancellation(
		ctx,
		extcontractcancellation.RequestCancellationInput{
			IdempotencyKey: *contractRow.CancellationFinancialCoreIdempotencyKey,
			TenantID:       contractRow.CancellationTenantID.String(),
			ContextID:      contractRow.CancellationContextID.String(),
			ContractID:     contractRow.ID,
			CustomerID:     contractRow.CustomerID,
			Reason:         *contractRow.CancellationReason,
		},
	)
	if err == nil {
		if result.Accepted {
			next := contractCancellationExternalAcceptedStep
			return saga.StepResult{
				Outcome:  saga.StepOutcomeCompleted,
				NextStep: &next,
			}, nil
		}

		reason := "external cancellation request was definitively rejected"
		return saga.StepResult{Outcome: saga.StepOutcomeCompensate, Reason: &reason}, nil
	}

	if errors.Is(err, context.Canceled) {
		return saga.StepResult{}, err
	}

	switch {
	case errors.Is(err, context.DeadlineExceeded),
		errors.Is(err, extcontractcancellation.ErrRetryableFailure):
		retryAt := s.clock.Now().UTC().Add(
			cancellationSagaRetryDelay(
				instance.StepAttemptCount,
				s.retryBaseDelay,
				s.retryMaxDelay,
			),
		)

		reason := boundedCancellationSagaReason(err.Error())

		return saga.StepResult{
			Outcome: saga.StepOutcomeRetry,
			RetryAt: &retryAt,
			Reason:  &reason,
		}, nil

	case errors.Is(err, extcontractcancellation.ErrPermanentFailure),
		errors.Is(err, extcontractcancellation.ErrNotSupported):
		reason := boundedCancellationSagaReason(err.Error())

		return saga.StepResult{
			Outcome: saga.StepOutcomeCompensate,
			Reason:  &reason,
		}, nil

	case errors.Is(err, extcontractcancellation.ErrAmbiguousFailure):
		reason := boundedCancellationSagaReason(err.Error())

		return saga.StepResult{
			Outcome: saga.StepOutcomeReconcile,
			Reason:  &reason,
		}, nil

	default:
		// An unclassified external error is not sufficient evidence for a safe
		// compensation. Stop automatic progress and require reconciliation rather
		// than guessing whether the external side effect happened.
		reason := boundedCancellationSagaReason(err.Error())

		return saga.StepResult{
			Outcome: saga.StepOutcomeReconcile,
			Reason:  &reason,
		}, nil
	}
}

type cancellationExternalAcceptedStep struct {
	profiles      repo.CustomerProfile
	contract      repo.Contract
	creditAccount repo.CreditAccount
	clock         clock.Clock
}

var _ saga.TransactionalStep = (*cancellationExternalAcceptedStep)(nil)

// Name returns the saga step name.
func (s *cancellationExternalAcceptedStep) Name() model.SagaStep {
	return contractCancellationExternalAcceptedStep
}

// ExecuteTx applies the successful authoritative synchronous Financial Core result
// to local state. The external call already completed outside this transaction;
// this step only finalizes canceling -> canceled atomically with Saga progress.
func (s *cancellationExternalAcceptedStep) ExecuteTx(
	ctx context.Context,
	tx repo.Tx,
	instance model.SagaInstance,
) (saga.StepResult, error) {
	contractID, err := cancellationSagaContractID(instance)
	if err != nil {
		return saga.StepResult{}, err
	}

	contractQ := s.contract.Tx(tx)
	if err := lockCancellationCustomer(ctx, tx, s.profiles, contractQ, contractID); err != nil {
		return saga.StepResult{}, err
	}
	contractRow, err := contractQ.LockByID(ctx, contractID)
	if err != nil {
		return saga.StepResult{}, err
	}

	if contractRow.Status != model.CancelingContractStatus {
		reason := fmt.Sprintf(
			"contract %s is %s while external cancellation acceptance is being recorded",
			contractID,
			contractRow.Status,
		)
		return saga.StepResult{
			Outcome: saga.StepOutcomeReconcile,
			Reason:  &reason,
		}, nil
	}

	now := s.clock.Now().UTC()

	// Financial Core has authoritatively accepted the cancellation, so the
	// remaining principal no longer consumes BNPL capacity. Release it in the
	// same transaction that finalizes the contract and Saga step.
	if contractRow.RemainingPrincipalAmountMinor > 0 {
		accountQ := s.creditAccount.Tx(tx)
		account, lockErr := accountQ.LockByID(ctx, contractRow.CreditAccountID)
		if lockErr != nil {
			return saga.StepResult{}, lockErr
		}

		if account.ExposureAmountMinor < contractRow.RemainingPrincipalAmountMinor {
			reason := fmt.Sprintf("contract %s remaining principal exceeds credit exposure", contractID)
			return saga.StepResult{Outcome: saga.StepOutcomeReconcile, Reason: &reason}, nil
		}

		if _, err = accountQ.ReduceExposureIfSufficient(ctx, account.ID, contractRow.RemainingPrincipalAmountMinor, now); err != nil {
			return saga.StepResult{}, err
		}

		if _, err = accountQ.AppendTransaction(
			ctx,
			account.ID,
			model.ExposureDecreasedCreditTransactionType,
			contractRow.RemainingPrincipalAmountMinor,
			contractRow.Currency,
			model.ContractCanceledCreditTransactionReason,
			model.ContractCreditTransactionReferenceType,
			contractRow.ID.UUID(),
			now,
			now,
		); err != nil {
			return saga.StepResult{}, err
		}
	}

	if _, err = contractQ.MarkCanceledAfterCancellation(
		ctx,
		contractID,
		now,
		now,
	); err != nil {
		return saga.StepResult{}, err
	}

	return saga.StepResult{Outcome: saga.StepOutcomeCompleted}, nil
}

func (s *cancellationExternalRequestStep) loadContract(
	ctx context.Context,
	instance model.SagaInstance,
) (model.Contract, error) {
	contractID, err := cancellationSagaContractID(instance)
	if err != nil {
		return model.Contract{}, err
	}

	var contractRow model.Contract
	err = s.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		contractQ := s.contract.Conn(conn)
		row, getErr := contractQ.GetByID(ctx, contractID)
		if getErr != nil {
			return getErr
		}

		contractRow = row
		return nil
	})
	return contractRow, err
}

func cancellationSagaContractID(instance model.SagaInstance) (model.ContractID, error) {
	const op = "contractuc.cancellationSagaContractID"

	if instance.SubjectType == nil || strings.TrimSpace(*instance.SubjectType) != contractCancellationSagaSubjectType {
		return model.ContractID{}, fmt.Errorf("%s: contract cancellation saga has invalid subject type", op)
	}

	if instance.SubjectID == nil {
		return model.ContractID{}, fmt.Errorf("%s: contract cancellation saga subject id is required", op)
	}

	contractID, err := model.ParseContractID(strings.TrimSpace(*instance.SubjectID))
	if err != nil {
		return model.ContractID{}, fmt.Errorf("%s: contract cancellation saga subject id is invalid: %w", op, err)
	}

	if contractID.IsZero() {
		return model.ContractID{}, fmt.Errorf("%s: contract cancellation saga subject id must not be nil", op)
	}

	return contractID, nil
}

func cancellationSagaRetryDelay(
	attemptCount int,
	base time.Duration,
	max time.Duration,
) time.Duration {
	if attemptCount < 1 {
		attemptCount = 1
	}

	delay := base
	for i := 1; i < attemptCount; i++ {
		if delay >= max/2 {
			return max
		}

		delay *= 2
	}

	if delay > max {
		return max
	}

	return delay
}

func boundedCancellationSagaReason(value string) string {
	const maxReasonLength = 2048
	value = strings.TrimSpace(value)
	if value == "" {
		value = "unknown cancellation saga failure"
	}

	if len(value) > maxReasonLength {
		value = value[:maxReasonLength]
	}

	return value
}

// WithCustomerSynchronization is wired before the executor starts. Cancellation
// replays read existing state without locking an existing Saga under a customer
// lock, so transactional Saga steps can safely acquire customer before contract.
func (d *CancellationSagaDefinition) WithCustomerSynchronization(profiles repo.CustomerProfile) *CancellationSagaDefinition {
	d.localStep.profiles = profiles
	d.acceptedStep.profiles = profiles
	return d
}
func lockCancellationCustomer(ctx context.Context, tx repo.Tx, profiles repo.CustomerProfile, contracts repo.ContractTxQueryer, id model.ContractID) error {
	if profiles == nil {
		return nil
	}

	contract, err := contracts.GetByID(ctx, id)
	if err != nil {
		return err
	}

	profileQ := profiles.Tx(tx)
	_, err = profileQ.LockByID(ctx, contract.CustomerID)
	return err
}
