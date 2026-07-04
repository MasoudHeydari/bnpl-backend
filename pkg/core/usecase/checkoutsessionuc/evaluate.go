package checkoutsessionuc

import (
	"context"
	"fmt"
	"strconv"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/event/checkoutlifecycle"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/decisionuc"
)

// checkoutSessionDecisionKeyPrefix namespaces the Decision idempotency key
// derived from a checkout session. The evaluation generation is appended so an
// expired offer can be re-evaluated without mutating Decision history: each
// generation is a new immutable Decision keyed separately.
const checkoutSessionDecisionKeyPrefix = "checkout-session:"

// evaluateSession creates or replays the personalized Decision for a checkout
// session and records the result on the session.
//
// decisionuc.Create owns its own transaction, so it must run outside any
// transaction this use case holds.
func (uc *UseCase) evaluateSession(
	ctx context.Context,
	session model.CheckoutSession,
	externalCustomerID model.ExternalCustomerID,
) (model.CheckoutSession, Offer, error) {
	session, err := uc.prepareEvaluationGeneration(ctx, session)
	if err != nil {
		return model.CheckoutSession{}, Offer{}, err
	}

	generation := session.EvaluationGeneration
	if generation < 1 {
		generation = 1
	}

	output, err := uc.decision.Create(ctx, decisionuc.CreateInput{
		CheckoutKey: checkoutSessionDecisionKey(
			session.ID,
			generation,
		),
		ExternalCustomerID: externalCustomerID,
		PurchaseContext: decisionuc.PurchaseContext{
			Channel:         model.OnlinePurchaseChannel,
			OnlineStoreID:   session.OnlineStoreID,
			MerchantOrderID: session.MerchantOrderID,
		},
		RequestedAmountMinor: session.RequestedAmountMinor,
		Currency:             session.Currency,
	})
	if err != nil {
		return model.CheckoutSession{}, Offer{}, err
	}

	offer := offerFromDecision(output)

	status := model.OpenCheckoutSessionStatus
	eventType := ""

	if offer.Outcome == DeniedOfferOutcome {
		status = model.UnavailableCheckoutSessionStatus
		eventType = checkoutlifecycle.EventTypeUnavailable
	}

	updated, err := uc.recordEvaluation(
		ctx,
		session.ID,
		generation,
		output.Decision.ID,
		status,
		eventType,
	)
	if err != nil {
		return model.CheckoutSession{}, Offer{}, err
	}

	// Durable CheckoutSession state wins over this caller's local Decision
	// projection. A concurrent denied evaluation may already have terminalized
	// the session while this caller still holds an approved CreateOutput.
	if updated.Status == model.UnavailableCheckoutSessionStatus {
		return updated, Offer{Outcome: DeniedOfferOutcome}, nil
	}

	return updated, offer, nil
}

// prepareEvaluationGeneration bumps the generation when the currently linked
// Decision has expired so the next Create uses a fresh CheckoutKey.
//
// Once a Contract is linked, the accepted Decision is immutable: expiry of the
// original offer must not clear decision_id or start a new evaluation.
func (uc *UseCase) prepareEvaluationGeneration(
	ctx context.Context,
	session model.CheckoutSession,
) (model.CheckoutSession, error) {
	if session.DecisionID == nil || session.DecisionID.IsZero() {
		return session, nil
	}

	if session.ContractID != nil && !session.ContractID.IsZero() {
		return session, nil
	}

	var decision model.Decision

	err := uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			decisionQ := uc.decisionRepo.Conn(conn)

			found, findErr := decisionQ.GetByID(ctx, *session.DecisionID)
			if findErr != nil {
				return findErr
			}

			decision = found

			return nil
		},
	)
	if err != nil {
		return model.CheckoutSession{}, err
	}

	now := uc.clock.Now().UTC()
	if decision.ExpiresAt == nil || decision.ExpiresAt.After(now) {
		return session, nil
	}

	return uc.bumpEvaluationGenerationCAS(ctx, session)
}

func checkoutSessionDecisionKey(id model.CheckoutSessionID, generation int) string {
	return checkoutSessionDecisionKeyPrefix + id.String() + ":g" + strconv.Itoa(generation)
}

func (uc *UseCase) recordEvaluation(
	ctx context.Context,
	id model.CheckoutSessionID,
	expectedGeneration int,
	decisionID model.DecisionID,
	status model.CheckoutSessionStatus,
	eventType string,
) (model.CheckoutSession, error) {
	var (
		session model.CheckoutSession
		expired bool
	)

	err := uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			return conn.Tx(
				ctx,
				func(ctx context.Context, tx repo.Tx) error {
					sessionQ := uc.checkoutSession.Tx(tx)

					locked, lockErr := sessionQ.LockByID(ctx, id)
					if lockErr != nil {
						return lockErr
					}

					now := uc.clock.Now().UTC()

					if locked.Status == model.UnavailableCheckoutSessionStatus {
						session = locked
						return nil
					}

					if locked.Status == model.OpenCheckoutSessionStatus &&
						locked.ContractID == nil &&
						locked.IsExpiredAt(now) {
						transitioned, statusErr := uc.transitionToExpired(ctx, tx, locked, now)
						if statusErr != nil {
							return statusErr
						}

						session = transitioned
						expired = true
						return nil
					}

					if !locked.Status.AllowsCustomerContinuation() {
						return checkoutSessionNotContinuableError(
							fmt.Errorf(
								"checkoutsessionuc.UseCase.recordEvaluation: checkout session status is %q",
								locked.Status,
							),
						)
					}

					if locked.EvaluationGeneration != expectedGeneration {
						return cerr.Conflict(fmt.Errorf("checkout evaluation was superseded before decision linkage"))
					}

					linked, linkErr := sessionQ.LinkDecision(
						ctx,
						locked.ID,
						decisionID,
						expectedGeneration,
						now,
					)
					if linkErr != nil {
						return linkErr
					}

					updated := linked
					statusChanged := false

					if linked.Status != status {
						if transitionErr := linked.Status.ValidateTransitionTo(status); transitionErr != nil {
							return cerr.InternalError(transitionErr)
						}

						statusUpdated, statusErr := sessionQ.UpdateStatus(ctx, linked.ID, status, now)
						if statusErr != nil {
							return statusErr
						}

						updated = statusUpdated
						statusChanged = true
					}

					if eventType != "" && statusChanged {
						if enqueueErr := uc.lifecycle.Enqueue(
							ctx,
							tx,
							updated,
							eventType,
							now,
						); enqueueErr != nil {
							return enqueueErr
						}
					}

					session = updated
					return nil
				},
			)
		},
	)
	if err != nil {
		return model.CheckoutSession{}, err
	}

	if expired {
		return model.CheckoutSession{}, checkoutSessionExpiredError()
	}

	return session, nil
}

// offerFromDecision projects merchant-safe offer economics from a Decision.
//
// Reason codes are dropped deliberately. A denied checkout reports only that
// financing is unavailable — no credit balances or installment schedule.
func offerFromDecision(output decisionuc.CreateOutput) Offer {
	outcome := offerOutcome(output.Decision.Outcome)
	if outcome == DeniedOfferOutcome {
		return Offer{
			Outcome: DeniedOfferOutcome,
		}
	}

	offer := Offer{
		Outcome:                        outcome,
		AvailableCreditMinor:           output.AvailableCreditMinor,
		RequestedAmountMinor:           output.RequestedAmountMinor,
		ApprovedFinancedAmountMinor:    output.ApprovedFinancedAmountMinor,
		RequiredDownPaymentAmountMinor: output.UpfrontAmountMinor,
		Currency:                       output.Currency,
		OfferExpiresAt:                 output.ExpiresAt,
		CheckoutAction:                 output.CheckoutAction,
		Installments:                   []OfferInstallment{},
	}

	if output.InstallmentOffer == nil {
		return offer
	}

	schedule := output.InstallmentOffer
	offer.InstallmentCount = schedule.InstallmentCount
	offer.PayableNowAmountMinor = schedule.PayableNowAmountMinor
	offer.TotalPayableAmountMinor = schedule.TotalPayableAmountMinor

	installments := make([]OfferInstallment, 0, len(schedule.Installments))
	for _, row := range schedule.Installments {
		installments = append(installments, OfferInstallment{
			InstallmentOrder: row.InstallmentOrder,
			DueAt:            row.DueAt,
			TotalAmountMinor: row.TotalAmountMinor,
		})
	}

	offer.Installments = installments

	return offer
}

// offerOutcome maps persisted decision outcomes onto the two-value checkout
// contract. Conditional (partial financing) approvals are approvals.
func offerOutcome(outcome model.DecisionOutcome) OfferOutcome {
	switch outcome {
	case model.ApprovedDecisionOutcome, model.ConditionalDecisionOutcome:
		return ApprovedOfferOutcome

	default:
		return DeniedOfferOutcome
	}
}
