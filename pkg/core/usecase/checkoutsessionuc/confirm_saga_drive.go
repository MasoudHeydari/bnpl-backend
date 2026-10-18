package checkoutsessionuc

import (
	"context"
	"errors"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/contractuc"
)

const confirmSagaDriveMaxSteps = 8

// driveConfirmSaga runs ExecuteByID until the Saga leaves the running forward
// path or the step budget is exhausted. HTTP Confirm is a low-latency first
// attempt; the background saga_executor remains the durable progress owner.
func (uc *UseCase) driveConfirmSaga(ctx context.Context, sagaID model.SagaID) error {
	const op = "checkoutsessionuc.UseCase.driveConfirmSaga"

	for i := 0; i < confirmSagaDriveMaxSteps; i++ {
		instance, loadErr := uc.loadConfirmationSaga(ctx, sagaID)
		if loadErr != nil {
			return fmt.Errorf("%s: load saga: %w", op, loadErr)
		}

		if instance.Phase != model.ForwardSagaPhase {
			return nil
		}

		switch instance.Status {
		case model.RunningSagaStatus:
			// Continue driving.

		case model.WaitingSagaStatus,
			model.ReconciliationRequiredSagaStatus,
			model.ManualReviewRequiredSagaStatus,
			model.CompletedSagaStatus,
			model.CompensatedSagaStatus,
			model.FailedSagaStatus:
			return nil

		default:
			return nil
		}

		if err := uc.confirmationSagaExecutor.ExecuteByID(ctx, sagaID); err != nil {
			if errors.Is(err, repo.ErrSagaNotRunnable) {
				return nil
			}

			if errors.Is(err, repo.ErrSagaVersionConflict) {
				return nil
			}

			return fmt.Errorf("%s: execute saga %s: %w", op, sagaID, err)
		}
	}

	return nil
}

func (uc *UseCase) loadConfirmationSaga(
	ctx context.Context,
	sagaID model.SagaID,
) (model.SagaInstance, error) {
	var instance model.SagaInstance
	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		sagaQ := uc.confirmationSagaRepo.Conn(conn)
		row, getErr := sagaQ.GetByID(ctx, sagaID)
		if getErr != nil {
			return getErr
		}

		instance = row
		return nil
	})

	return instance, err
}

func (uc *UseCase) findConfirmationSagaBySession(
	ctx context.Context,
	sessionID model.CheckoutSessionID,
) (model.SagaInstance, error) {
	var instance model.SagaInstance
	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		sagaQ := uc.confirmationSagaRepo.Conn(conn)
		row, getErr := sagaQ.GetByTypeAndBusinessKey(
			ctx,
			checkoutConfirmationSagaType,
			model.SagaBusinessKey(sessionID.String()),
		)
		if getErr != nil {
			return getErr
		}

		instance = row
		return nil
	})

	return instance, err
}

// resumeConfirmSaga loads the durable confirmation Saga for an unlinked
// confirming session and drives it forward.
func (uc *UseCase) resumeConfirmSaga(
	ctx context.Context,
	session model.CheckoutSession,
	input ConfirmInput,
) (ConfirmOutput, error) {
	const op = "checkoutsessionuc.UseCase.resumeConfirmSaga"

	instance, err := uc.findConfirmationSagaBySession(ctx, session.ID)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			// Orphan confirming without a Saga should not happen after atomic
			// start. Fail closed with IN_PROGRESS for operator diagnosis.
			return ConfirmOutput{}, checkoutPaymentInProgressError(
				fmt.Errorf("%s: confirming session %s has no confirmation saga", op, session.ID),
			)
		}

		return ConfirmOutput{}, err
	}

	if err := uc.driveConfirmSaga(ctx, instance.ID); err != nil {
		return ConfirmOutput{}, err
	}

	return uc.projectConfirmAfterSaga(ctx, session.ID, input)
}

func (uc *UseCase) projectConfirmAfterSaga(
	ctx context.Context,
	sessionID model.CheckoutSessionID,
	input ConfirmInput,
) (ConfirmOutput, error) {
	const op = "checkoutsessionuc.UseCase.projectConfirmAfterSaga"

	var session model.CheckoutSession
	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			sessionQ := uc.checkoutSession.Tx(tx)
			row, getErr := sessionQ.LockByID(ctx, sessionID)
			if getErr != nil {
				return getErr
			}

			session = row
			return nil
		})
	})
	if err != nil {
		return ConfirmOutput{}, err
	}

	instance, err := uc.findConfirmationSagaBySession(ctx, sessionID)
	if err != nil && !cerr.IsCode(err, cerr.CodeResourceNotFound) {
		return ConfirmOutput{}, err
	}

	if err == nil && instance.Status == model.CompensatedSagaStatus {
		// Deterministic prepare rejection reopened the claim; refresh offer.
		return uc.refreshAfterDeterministicContractReject(ctx, session, input)
	}

	if session.Status == model.ConfirmingCheckoutSessionStatus && session.ContractID == nil {
		return ConfirmOutput{}, checkoutPaymentInProgressError(
			fmt.Errorf("%s: confirmation still in progress for session %s", op, sessionID),
		)
	}

	if session.ContractID == nil {
		return ConfirmOutput{}, checkoutPaymentInProgressError(
			fmt.Errorf("%s: confirmation has not linked a contract yet", op),
		)
	}

	return uc.replayConfirm(ctx, session, input)
}

func (uc *UseCase) loadPreparedCheckoutOutput(
	ctx context.Context,
	session model.CheckoutSession,
) (contractuc.CreateContractOutput, error) {
	const op = "checkoutsessionuc.UseCase.loadPreparedCheckoutOutput"

	if session.ContractID == nil {
		return contractuc.CreateContractOutput{}, fmt.Errorf("%s: session has no contract", op)
	}

	var contractRow model.Contract
	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		contractQ := uc.contract.Conn(conn)
		row, getErr := contractQ.GetByID(ctx, *session.ContractID)
		if getErr != nil {
			return getErr
		}

		contractRow = row
		return nil
	})
	if err != nil {
		return contractuc.CreateContractOutput{}, err
	}

	paymentRequired, modeErr := uc.contractRequiresInitialPayment(ctx, contractRow)
	if modeErr != nil {
		return contractuc.CreateContractOutput{}, modeErr
	}

	out := contractuc.CreateContractOutput{
		Contract:        contractRow,
		PaymentRequired: paymentRequired,
	}
	if paymentRequired {
		attempt, attemptErr := uc.loadCheckoutPaymentAttempt(ctx, contractRow.ID)
		if attemptErr != nil {
			return contractuc.CreateContractOutput{}, attemptErr
		}

		out.PaymentAttempt = attempt
		out.Payment = contractuc.CheckoutPaymentProjection(contractRow)
	}

	return out, nil
}
