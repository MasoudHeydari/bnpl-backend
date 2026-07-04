package contractuc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

const maxContractCancellationIdempotencyKeyLength = 256

// CancelInput carries customer-initiated contract cancellation start inputs.
//
// IdentitySubjectID is the authenticated Identity Service subject ID
// (x-auth-subject-id / customer_profiles.identity_subject_id). Ownership checks
// use the resolved customer_profiles.id against contracts.customer_id.
type CancelInput struct {
	// TenantID identifies the tenant.
	TenantID model.TenantID
	// ContextID identifies the payment context.
	ContextID model.ContextID

	// ContractID identifies the contract.
	ContractID model.ContractID
	// IdentitySubjectID is the Identity Service subject mapped to customer_profiles.identity_subject_id.
	IdentitySubjectID model.IdentitySubjectID

	// IdempotencyKey is the client idempotency key for the operation.
	IdempotencyKey string
	// Reason describes why the error or outcome occurred.
	Reason model.ContractCancellationReason
}

// CancelOutput reports contract cancellation start results.
type CancelOutput struct {
	// Result reports the use-case outcome classification.
	Result CancelResult
	// Contract is the activated or affected contract.
	Contract model.Contract
}

// Cancel starts cancellation for an active contract (active → canceling).
//
// The local canceling transition, Saga creation, forward-step completion, and
// audit record are committed atomically. After commit, the request path
// performs one low-latency Saga execution attempt for the external handoff.
// Retry, compensation, and crash recovery remain durable and are resumed by the
// generic Saga background job. No external call runs inside the DB transaction.
//
// Financial Core is called synchronously outside the local transaction. A
// successful authoritative response is then finalized locally as
// canceling → canceled. This command never invents monetary truth.
func (uc *UseCase) Cancel(
	ctx context.Context,
	input CancelInput,
) (CancelOutput, error) {
	const op = "contractuc.UseCase.Cancel"

	if err := validateCancelInput(input); err != nil {
		return CancelOutput{}, err
	}

	now := uc.clock.Now().UTC()

	var output CancelOutput
	var startedSagaID model.SagaID

	err := uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			return conn.Tx(
				ctx,
				func(ctx context.Context, tx repo.Tx) error {
					customerProfileQ := uc.customerProfile.Tx(tx)
					contractQ := uc.contract.Tx(tx)
					paymentAttemptQ := uc.paymentAttempt.Tx(tx)
					outboxQ := uc.outbox.Tx(tx)

					profile, profileErr := customerProfileQ.LockByIdentitySubjectID(
						ctx,
						input.IdentitySubjectID,
					)
					if profileErr != nil {
						if cerr.IsCode(profileErr, cerr.CodeResourceNotFound) {
							return contractDoesNotBelongToCustomerError(profileErr)
						}

						return profileErr
					}

					customerID := profile.ID

					contractRow, lockErr := contractQ.LockByID(ctx, input.ContractID)
					if lockErr != nil {
						if cerr.IsCode(lockErr, cerr.CodeResourceNotFound) {
							return contractNotFoundError(lockErr)
						}

						return lockErr
					}

					if contractRow.CustomerID != customerID {
						return contractDoesNotBelongToCustomerError(
							fmt.Errorf("%s: contract does not belong to customer profile", op),
						)
					}

					fingerprint := cancellationRequestFingerprint(
						input.ContractID,
						customerID,
						input.Reason,
						input.TenantID,
						input.ContextID,
					)
					idempotencyKey := strings.TrimSpace(input.IdempotencyKey)
					fcIdempotencyKey := financialCoreCancellationIdempotencyKey(input.ContractID)

					switch contractRow.Status {
					case model.ActiveContractStatus:
						if err := contractRow.Status.ValidateTransitionTo(
							model.CancelingContractStatus,
						); err != nil {
							return contractStateDoesNotAllowCancellationError(contractRow.Status)
						}

						hasBlockingAttempt, attemptErr := paymentAttemptQ.HasBlockingAttemptForContract(
							ctx, contractRow.ID,
						)
						if attemptErr != nil {
							return attemptErr
						}

						if hasBlockingAttempt {
							return contractCancellationBlockedByPaymentAttemptError()
						}

						// Quiesce installment lifecycle before committing canceling. Lifecycle
						// jobs lock installment rows; waiting here ensures any already-claimed
						// due/overdue transition finishes before cancellation becomes visible.
						installmentQ := uc.installment.Tx(tx)
						if _, lockInstallmentsErr := installmentQ.LockByContractID(
							ctx, contractRow.ID,
						); lockInstallmentsErr != nil {
							return lockInstallmentsErr
						}

						var (
							updated model.Contract
							markErr error
						)
						updated, startedSagaID, markErr = uc.startContractCancellationSagaTx(
							ctx,
							tx,
							contractRow,
							input,
							fingerprint,
							idempotencyKey,
							fcIdempotencyKey,
							now,
						)
						if markErr != nil {
							if cerr.IsCode(markErr, cerr.CodeConflict) {
								// Concurrent cancel won the CAS. Re-lock and treat as
								// an in-progress cancellation for deterministic replay.
								relocked, relockErr := contractQ.LockByID(ctx, input.ContractID)
								if relockErr != nil {
									return relockErr
								}

								return uc.resolveCancelAgainstExistingState(
									relocked,
									fingerprint,
									idempotencyKey,
									&output,
								)
							}

							return markErr
						}

						if err := uc.insertCancelAuditOutbox(
							ctx,
							outboxQ,
							cancelAuditFacts{
								IdentitySubjectID: input.IdentitySubjectID,
								Contract:          updated,
								PreviousStatus:    model.ActiveContractStatus,
								Reason:            input.Reason,
							},
							now,
						); err != nil {
							return err
						}

						output = CancelOutput{
							Result:   CancelResultStarted,
							Contract: updated,
						}
						return nil

					case model.CancelingContractStatus, model.CanceledContractStatus:
						return uc.resolveCancelAgainstExistingState(
							contractRow,
							fingerprint,
							idempotencyKey,
							&output,
						)

					default:
						return contractStateDoesNotAllowCancellationError(contractRow.Status)
					}
				},
			)
		},
	)
	if err != nil {
		return CancelOutput{}, err
	}

	if output.Result != CancelResultStarted {
		return output, nil
	}

	// The local active -> canceling transition and Saga state are already
	// committed. ExecuteByID is only a low-latency first attempt; retry or
	// compensation remains durable and will be resumed by the background job.
	if err := uc.cancellationSagaExecutor.ExecuteByID(ctx, startedSagaID); err != nil {
		return CancelOutput{}, contractCancellationFinancialCoreUnavailableError(err)
	}

	// ExecuteByID advances one durable step at a time. After an authoritative
	// synchronous Financial Core success, the Saga is now positioned on the
	// local transactional finalizer. Run that finalizer immediately so the API
	// can return the terminal canceled contract without waiting for the job.
	if err := uc.finalizeSuccessfulCancellationSaga(ctx, startedSagaID, &output); err != nil {
		return CancelOutput{}, err
	}

	return output, nil
}

func (uc *UseCase) finalizeSuccessfulCancellationSaga(
	ctx context.Context,
	sagaID model.SagaID,
	output *CancelOutput,
) error {
	var instance model.SagaInstance
	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		cancellationSagaRepoQ := uc.cancellationSagaRepo.Conn(conn)
		row, err := cancellationSagaRepoQ.GetByID(ctx, sagaID)
		if err != nil {
			return err
		}

		instance = row
		return nil
	})
	if err != nil {
		return err
	}

	if instance.Phase != model.ForwardSagaPhase ||
		instance.Status != model.RunningSagaStatus ||
		instance.CurrentStep != contractCancellationExternalAcceptedStep {
		return nil
	}

	if err := uc.cancellationSagaExecutor.ExecuteByID(ctx, sagaID); err != nil {
		return contractCancellationFinancialCoreUnavailableError(err)
	}

	return uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		contractQ := uc.contract.Conn(conn)
		contractRow, err := contractQ.GetByID(ctx, output.Contract.ID)
		if err != nil {
			return err
		}

		output.Contract = contractRow
		if contractRow.Status == model.CanceledContractStatus {
			output.Result = CancelResultCompleted
		}

		return nil
	})
}

func (uc *UseCase) resolveCancelAgainstExistingState(
	contractRow model.Contract,
	fingerprint string,
	idempotencyKey string,
	output *CancelOutput,
) error {
	const op = "contractuc.UseCase.resolveCancelAgainstExistingState"

	storedKey := optionalString(contractRow.CancellationIdempotencyKey)
	storedFingerprint := optionalString(contractRow.CancellationRequestFingerprint)

	switch contractRow.Status {
	case model.CancelingContractStatus:
		if storedKey == "" {
			return contractStateDoesNotAllowCancellationError(contractRow.Status)
		}

		if storedKey != idempotencyKey {
			return contractCancellationAlreadyInProgressError()
		}

		if storedFingerprint != fingerprint {
			return cancellationIdempotencyKeyConflictError(
				fmt.Errorf("%s: idempotency key reused with different cancellation inputs", op),
			)
		}

		*output = CancelOutput{
			Result:   CancelResultAlreadyCanceling,
			Contract: contractRow,
		}
		return nil

	case model.CanceledContractStatus:
		if storedKey == "" || storedKey != idempotencyKey {
			return contractStateDoesNotAllowCancellationError(contractRow.Status)
		}

		if storedFingerprint != fingerprint {
			return cancellationIdempotencyKeyConflictError(
				fmt.Errorf("%s: idempotency key reused with different cancellation inputs", op),
			)
		}

		*output = CancelOutput{
			Result:   CancelResultAlreadyCanceled,
			Contract: contractRow,
		}
		return nil

	default:
		return contractStateDoesNotAllowCancellationError(contractRow.Status)
	}
}

func validateCancelInput(input CancelInput) error {
	if input.TenantID.IsZero() {
		return tenantIDRequiredError()
	}

	if input.ContextID.IsZero() {
		return contextIDRequiredError()
	}

	if input.ContractID.IsZero() {
		return contractIDRequiredError()
	}

	if input.IdentitySubjectID.IsZero() {
		return identitySubjectIDRequiredError()
	}

	idempotencyKey := strings.TrimSpace(input.IdempotencyKey)
	if idempotencyKey == "" || len(idempotencyKey) > maxContractCancellationIdempotencyKeyLength {
		return cancellationIdempotencyKeyRequiredError()
	}

	if !input.Reason.IsValid() {
		return cancellationReasonInvalidError()
	}

	return nil
}

func cancellationRequestFingerprint(
	contractID model.ContractID,
	customerID model.CustomerID,
	reason model.ContractCancellationReason,
	tenantID model.TenantID,
	contextID model.ContextID,
) string {
	raw := strings.Join([]string{
		"contract_cancellation",
		contractID.String(),
		customerID.String(),
		reason.String(),
		tenantID.String(),
		contextID.String(),
	}, "|")

	sum := sha256.Sum256([]byte(raw))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func financialCoreCancellationIdempotencyKey(contractID model.ContractID) string {
	return fmt.Sprintf("bnpl:contract:%s:cancellation", contractID)
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}
