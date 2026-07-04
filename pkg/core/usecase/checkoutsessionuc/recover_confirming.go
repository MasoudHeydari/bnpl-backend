package checkoutsessionuc

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/contractuc"
)

// DefaultConfirmingClaimTimeout is the delay before recovery inspection.
// It is NOT a lease expiry or proof that an in-flight Confirm has stopped;
// the scanner never steals or reopens unlinked claims.
const DefaultConfirmingClaimTimeout = 15 * time.Minute

// RecoverStaleConfirmingOutput reports how many stale confirming claims one
// sweep reconciled.
type RecoverStaleConfirmingOutput struct {
	// RelinkedCount is how many sessions were linked to an already-persisted
	// Contract that Confirm never finished attaching.
	RelinkedCount int

	// UnresolvedCount is how many abandoned-looking claims cannot safely be
	// released: an uncommitted Contract may still be in progress on another
	// replica. They require fencing/operational reconciliation, not a TTL reset.
	UnresolvedCount int

	// ResumedCount counts durable processing claims whose FC recognition was
	// retried or finalized after a crash.
	ResumedCount int
}

// RecoverStaleConfirmingSessions reconciles unlinked confirming sessions and
// outstanding activation/recognized-contract processing sessions. The scan
// locks rows only for local state recovery; FC is called AFTER the commit.
//
// An unlinked confirming claim cannot be released merely because no Contract
// is visible: the original worker may be paused before its commit. Without a
// durable fencing token a TTL reset is unsafe. Such claims remain confirming
// for explicit diagnosis and cannot be canceled or re-financed. Existing
// persisted Contracts are relinked idempotently and submitted/recognized with
// their stable FC source reference.
func (uc *UseCase) RecoverStaleConfirmingSessions(ctx context.Context) (RecoverStaleConfirmingOutput, error) {
	const op = "checkoutsessionuc.UseCase.RecoverStaleConfirmingSessions"

	var output RecoverStaleConfirmingOutput
	var resume []model.Contract

	now := uc.clock.Now().UTC()
	staleBefore := now.Add(-DefaultConfirmingClaimTimeout)

	err := uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			return conn.Tx(
				ctx,
				func(ctx context.Context, tx repo.Tx) error {
					sessionQ := uc.checkoutSession.Tx(tx)
					contractQ := uc.contract.Tx(tx)

					candidates, listErr := sessionQ.ReopenStaleConfirmingWithoutContract(
						ctx, now, staleBefore, uc.expiryBatchSize,
					)
					if listErr != nil {
						return fmt.Errorf("%s: list stale confirming sessions: %w", op, listErr)
					}

					for _, session := range candidates {
						if session.Status == model.ProcessingCheckoutSessionStatus {
							if session.ContractID == nil {
								output.UnresolvedCount++
								continue
							}
							contract, findErr := contractQ.GetByID(ctx, *session.ContractID)
							if findErr != nil {
								return fmt.Errorf("%s: load processing contract: %w", op, findErr)
							}
							// Refresh the scan timestamp in the same short TX as the
							// SKIP LOCKED claim. Other replicas cannot pick this row
							// again until the recovery timeout; FC calls remain outside.
							if _, touchErr := sessionQ.UpdateStatus(ctx, session.ID, session.Status, now); touchErr != nil {
								return touchErr
							}
							resume = append(resume, contract)
							continue
						}

						if session.DecisionID == nil || session.DecisionID.IsZero() {
							output.UnresolvedCount++
							continue
						}
						existing, findErr := contractQ.GetByDecisionID(ctx, *session.DecisionID)
						if findErr != nil {
							if cerr.IsCode(findErr, cerr.CodeResourceNotFound) {
								// No committed Contract does not prove the original
								// owner stopped. Never reopen or create another one.
								output.UnresolvedCount++
								continue
							}
							return fmt.Errorf("%s: lookup contract by decision: %w", op, findErr)
						}
						if relinkErr := uc.relinkRecoveredContract(ctx, tx, session, existing, now); relinkErr != nil {
							return fmt.Errorf("%s: relink recovered contract: %w", op, relinkErr)
						}
						output.RelinkedCount++
						// Both initial-payment and merchant-funding contracts may
						// need resume; the post-commit loop selects by policy.
						resume = append(resume, existing)
					}

					return nil
				},
			)
		},
	)
	if err != nil {
		return RecoverStaleConfirmingOutput{}, err
	}

	// Resume FC recognition using each contract's stable source reference.
	// One unavailable FC call must not starve the rest of this claimed batch.
	// Keep reporting the first failure to the task runtime while allowing
	// other independent sessions to finish their reconciliation.
	var firstResumeErr error
	for _, contract := range resume {
		// payment_pending alone does not choose the fulfillment path: initial-
		// payment contracts wait for PaymentAttempt/provider results, while
		// merchant-funding contracts wait for RecordMerchantEntitlement.
		merchantFunding, modeErr := uc.isMerchantFundingContract(ctx, contract)
		if modeErr != nil {
			output.UnresolvedCount++
			if firstResumeErr == nil {
				firstResumeErr = fmt.Errorf("%s: resolve activation policy: %w", op, modeErr)
			}

			continue
		}

		if !merchantFunding {
			// Initial-payment Contracts are owned by payment-result
			// reconciliation. Never invent a payment result or switch them to
			// merchant entitlement recognition; count them as unresolved here so
			// operators can see the scanner left them alone.
			output.UnresolvedCount++

			continue
		}

		if contract.Status == model.ActiveContractStatus && hasFinancialRecognition(contract) {
			if _, completeErr := uc.completeRecognizedCheckout(ctx, contract); completeErr != nil {
				output.UnresolvedCount++
				if firstResumeErr == nil {
					firstResumeErr = fmt.Errorf("%s: complete recognized checkout: %w", op, completeErr)
				}

				continue
			}

			output.ResumedCount++

			continue
		}

		if contract.Status != model.PaymentPendingContractStatus && contract.Status != model.ActiveContractStatus {
			output.UnresolvedCount++

			continue
		}

		// Merchant-funding fulfillment is retried using the same contract source
		// reference. Ambiguous FC timeouts leave PROCESSING durable.
		prepared := contractuc.CreateContractOutput{Contract: contract, PaymentRequired: false}
		activated, submitErr := uc.contractUC.SubmitPreparedCheckoutFulfillment(ctx, prepared)
		if submitErr != nil {
			output.UnresolvedCount++
			if firstResumeErr == nil {
				firstResumeErr = fmt.Errorf("%s: resume financial recognition: %w", op, submitErr)
			}

			continue
		}

		if _, completeErr := uc.completeRecognizedCheckout(ctx, activated.Contract); completeErr != nil {
			output.UnresolvedCount++
			if firstResumeErr == nil {
				firstResumeErr = fmt.Errorf("%s: complete recognized checkout: %w", op, completeErr)
			}

			continue
		}

		output.ResumedCount++
	}

	return output, firstResumeErr
}

func (uc *UseCase) relinkRecoveredContract(
	ctx context.Context,
	tx repo.Tx,
	session model.CheckoutSession,
	contract model.Contract,
	now time.Time,
) error {
	sessionQ := uc.checkoutSession.Tx(tx)

	linked, err := sessionQ.LinkContract(ctx, session.ID, contract.ID, now)
	if err != nil {
		return err
	}

	// Even an ACTIVE Contract must not bypass recognition/notification
	// reconciliation. The post-commit resume step verifies FC evidence and
	// performs the merchant-visible completion exactly once.
	if linked.Status == model.ProcessingCheckoutSessionStatus {
		return nil
	}
	if err := linked.Status.ValidateTransitionTo(model.ProcessingCheckoutSessionStatus); err != nil {
		return cerr.InternalError(err)
	}
	_, err = sessionQ.UpdateStatus(ctx, linked.ID, model.ProcessingCheckoutSessionStatus, now)
	if err != nil {
		return err
	}

	return nil
}
