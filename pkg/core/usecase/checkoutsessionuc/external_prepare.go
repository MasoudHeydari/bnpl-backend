package checkoutsessionuc

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/event/checkoutlifecycle"
	"github.com/parstis/bnpl-backend/pkg/core/external/customer"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// prepareExternalOffer resolves the Merchant-asserted phone through the
// Customer Reference Service (when not already bound), binds the resolved
// customer, and creates or replays the financing Decision/Offer. It never sends
// OTP, creates a Contract, creates a PaymentAttempt, or calls Financial Core for
// money movement.
//
// Network Customer Reference resolution runs outside any DB transaction. An
// idempotent CreatePartner retry resumes preparation when the session exists but
// the Decision is still missing, and replays a durable prepared offer without
// duplicating Decision rows.
//
// Binding a customer here is Customer Reference resolution
// (phone P → external customer_id → BNPL CustomerID), not OTP verification or
// purchase authorization.
func (uc *UseCase) prepareExternalOffer(
	ctx context.Context,
	session model.CheckoutSession,
) (model.CheckoutSession, Offer, error) {
	const op = "checkoutsessionuc.UseCase.prepareExternalOffer"

	if session.Origin != model.ExternalPartnerCheckoutSessionOrigin {
		return model.CheckoutSession{}, Offer{}, cerr.InternalError(
			fmt.Errorf("%s: expected external partner origin, got %q", op, session.Origin),
		)
	}

	// Terminal / non-continuable sessions are returned as-is for Partner Create
	// replay (for example unavailable after a denied Decision).
	if !session.Status.AllowsCustomerContinuation() {
		return session, Offer{}, nil
	}

	if session.ContractID != nil {
		return session, Offer{}, nil
	}

	now := uc.clock.Now().UTC()
	if session.IsExpiredAt(now) {
		expired, err := uc.persistExternalExpired(ctx, session.PublicToken)
		if err != nil {
			return model.CheckoutSession{}, Offer{}, err
		}

		return expired, Offer{}, checkoutSessionExpiredError()
	}

	// Already prepared: replay without Customer Reference / Decision recreation.
	if session.CustomerID != nil && session.DecisionID != nil && !session.DecisionID.IsZero() {
		prepared, offer, err := uc.loadPreparedExternalOffer(ctx, session, now)
		if err != nil {
			return model.CheckoutSession{}, Offer{}, err
		}

		clamped, clampErr := uc.clampExternalSessionToOffer(ctx, prepared, offer)
		if clampErr != nil {
			return model.CheckoutSession{}, Offer{}, clampErr
		}

		return clamped, offer, nil
	}

	phone, err := uc.decryptCustomerPhone(session)
	if err != nil {
		return model.CheckoutSession{}, Offer{}, err
	}

	identitySubjectID, usable, err := uc.resolveExternalIdentitySubjectID(ctx, session, phone)
	if err != nil {
		return model.CheckoutSession{}, Offer{}, err
	}

	if !usable {
		// Customer Reference NOT_FOUND or suspended is a merchant-safe financing
		// outcome, not a retryable dependency failure. Terminalize the
		// already-created session so it cannot remain an orphan active attempt.
		unavailable, unavailableErr := uc.persistExternalUnavailable(ctx, session.PublicToken)
		if unavailableErr != nil {
			return model.CheckoutSession{}, Offer{}, unavailableErr
		}

		return unavailable, Offer{}, nil
	}

	bound, err := uc.bindExternalCustomer(ctx, session.PublicToken, identitySubjectID)
	if err != nil {
		return model.CheckoutSession{}, Offer{}, err
	}

	prepared, offer, err := uc.prepareOfferForBoundSession(ctx, bound, identitySubjectID)
	if err != nil {
		return model.CheckoutSession{}, Offer{}, err
	}

	clamped, clampErr := uc.clampExternalSessionToOffer(ctx, prepared, offer)
	if clampErr != nil {
		return model.CheckoutSession{}, Offer{}, clampErr
	}

	return clamped, offer, nil
}

// resolveExternalIdentitySubjectID returns the already-bound external customer
// id, or resolves the merchant-asserted mobile through the Customer Reference
// Service.
//
// usable=false means the checkout must become merchant-safe unavailable
// (NOT_FOUND or suspended). Unknown or unspecified Customer Reference statuses
// fail closed as integration/protocol errors and never bind a customer.
// Customer Reference status is never written into CustomerProfile.status.
func (uc *UseCase) resolveExternalIdentitySubjectID(
	ctx context.Context,
	session model.CheckoutSession,
	phone string,
) (model.IdentitySubjectID, bool, error) {
	const op = "checkoutsessionuc.UseCase.resolveExternalIdentitySubjectID"

	if session.CustomerID != nil {
		var profileID model.IdentitySubjectID

		err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
			customerQ := uc.customerProfile.Conn(conn)
			profile, getErr := customerQ.GetByID(ctx, *session.CustomerID)
			if getErr != nil {
				return getErr
			}

			profileID = profile.IdentitySubjectID
			return nil
		})
		if err != nil {
			return model.IdentitySubjectID{}, false, fmt.Errorf("%s: load bound customer: %w", op, err)
		}

		return profileID, true, nil
	}

	resolved, err := uc.customerResolver.ResolveByMobile(
		ctx,
		customer.ResolveByMobileInput{MobileE164: phone},
	)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return model.IdentitySubjectID{}, false, nil
		}

		return model.IdentitySubjectID{}, false, fmt.Errorf("%s: resolve customer by mobile: %w", op, err)
	}

	if resolved.ExternalCustomerID.IsZero() {
		return model.IdentitySubjectID{}, false, cerr.InternalError(
			fmt.Errorf("%s: customer reference service returned an empty customer id", op),
		)
	}

	switch resolved.Status {
	case customer.ActiveStatus:
		return resolved.ExternalCustomerID, true, nil

	case customer.SuspendedStatus:
		return model.IdentitySubjectID{}, false, nil

	default:
		return model.IdentitySubjectID{}, false, cerr.InternalError(
			fmt.Errorf("%s: unsupported customer reference status", op),
		)
	}
}

// loadPreparedExternalOffer checks the durable Decision linked to session for
// CreatePartner replay. An expired initial offer does not regenerate terms:
// the checkout is expired. Full Hosted offer economics are projected by GET.
func (uc *UseCase) loadPreparedExternalOffer(
	ctx context.Context,
	session model.CheckoutSession,
	now time.Time,
) (model.CheckoutSession, Offer, error) {
	const op = "checkoutsessionuc.UseCase.loadPreparedExternalOffer"

	var decision model.Decision

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		decisionQ := uc.decisionRepo.Conn(conn)
		found, getErr := decisionQ.GetByID(ctx, *session.DecisionID)
		if getErr != nil {
			return getErr
		}

		decision = found

		return nil
	})
	if err != nil {
		return model.CheckoutSession{}, Offer{}, fmt.Errorf("%s: load decision: %w", op, err)
	}

	outcome := offerOutcome(decision.Outcome)
	offer := Offer{Outcome: outcome, OfferExpiresAt: decision.ExpiresAt}

	if outcome == DeniedOfferOutcome {
		return session, offer, nil
	}

	if decision.ExpiresAt != nil && !decision.ExpiresAt.After(now) {
		expired, expireErr := uc.persistExternalExpired(ctx, session.PublicToken)
		if expireErr != nil {
			return model.CheckoutSession{}, Offer{}, expireErr
		}

		return expired, Offer{}, checkoutSessionExpiredError()
	}

	return session, offer, nil
}

// projectPersistedOffer rebuilds the Hosted offer from durable Decision rows.
func (uc *UseCase) projectPersistedOffer(
	ctx context.Context,
	session model.CheckoutSession,
) (Offer, error) {
	const op = "checkoutsessionuc.UseCase.projectPersistedOffer"

	if session.DecisionID == nil || session.DecisionID.IsZero() {
		return Offer{}, checkoutOfferNotReadyError(
			fmt.Errorf("%s: checkout session has no linked decision", op),
		)
	}

	var offer Offer

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		decisionQ := uc.decisionRepo.Conn(conn)
		allocationQ := uc.checkoutDecisionAllocation.Conn(conn)
		installmentQ := uc.installment.Conn(conn)
		installmentChargeQ := uc.installmentCharge.Conn(conn)

		decision, decisionErr := decisionQ.GetByID(ctx, *session.DecisionID)
		if decisionErr != nil {
			return decisionErr
		}

		outcome := offerOutcome(decision.Outcome)
		if outcome == DeniedOfferOutcome {
			offer = Offer{Outcome: DeniedOfferOutcome}

			return nil
		}

		allocation, allocationErr := allocationQ.GetByDecisionID(ctx, *session.DecisionID)
		if allocationErr != nil {
			return allocationErr
		}

		installments, listErr := installmentQ.ListByDecisionID(ctx, *session.DecisionID)
		if listErr != nil {
			return listErr
		}

		charges, chargeErr := installmentChargeQ.ListByInstallmentIDs(ctx, installmentIDsOf(installments))
		if chargeErr != nil {
			return chargeErr
		}

		selected, resolveErr := uc.checkoutPolicies.Resolve(policy.Ref{
			ID:      decision.PolicyID,
			Version: decision.PolicyVersion,
		})
		if resolveErr != nil {
			return fmt.Errorf("%s: resolve checkout policy: %w", op, resolveErr)
		}

		projected, projectErr := offerFromPersistedDecision(
			decision,
			session,
			allocation,
			installments,
			charges,
			selected,
		)
		if projectErr != nil {
			return fmt.Errorf("%s: project persisted offer: %w", op, projectErr)
		}

		offer = projected

		return nil
	})
	if err != nil {
		return Offer{}, err
	}

	return offer, nil
}

func installmentIDsOf(installments []model.Installment) []model.InstallmentID {
	ids := make([]model.InstallmentID, 0, len(installments))
	for _, row := range installments {
		ids = append(ids, row.ID)
	}

	return ids
}

// clampExternalSessionToOffer shortens the open checkout lifetime to the offer
// expiry when the session would otherwise outlive the prepared terms.
//
// ClampExpiresAt uses LEAST and never extends expires_at. After the mutation
// returns, expiry is re-checked with a fresh clock sample so a lock wait that
// crossed the newly-clamped deadline cannot leave an open+elapsed durable row.
// When expiry wins, open → expired and the lifecycle event commit atomically
// with the clamp; the semantic expired error is returned only after commit.
func (uc *UseCase) clampExternalSessionToOffer(
	ctx context.Context,
	session model.CheckoutSession,
	offer Offer,
) (model.CheckoutSession, error) {
	if offer.Outcome != ApprovedOfferOutcome || offer.OfferExpiresAt == nil {
		return session, nil
	}

	if !session.ExpiresAt.After(*offer.OfferExpiresAt) {
		return session, nil
	}

	var (
		clamped model.CheckoutSession
		expired bool
	)

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			sessionQ := uc.checkoutSession.Tx(tx)
			clampAt := uc.clock.Now().UTC()

			updated, clampErr := sessionQ.ClampExpiresAt(
				ctx, session.ID, *offer.OfferExpiresAt, clampAt,
			)
			if clampErr != nil {
				return clampErr
			}

			// ClampExpiresAt may have waited for the row lock past the new
			// deadline. Evaluate eligibility only after the mutation returns.
			now := uc.clock.Now().UTC()

			if updated.Status == model.OpenCheckoutSessionStatus &&
				updated.ContractID == nil &&
				updated.IsExpiredAt(now) {
				transitioned, statusErr := uc.transitionToExpired(ctx, tx, updated, now)
				if statusErr != nil {
					return statusErr
				}

				clamped = transitioned
				expired = true

				return nil
			}

			clamped = updated

			return nil
		})
	})
	if err != nil {
		return model.CheckoutSession{}, err
	}

	if expired {
		return clamped, checkoutSessionExpiredError()
	}

	return clamped, nil
}

// persistExternalUnavailable terminalizes a deterministic Identity miss.
//
// Identity resolution happens after the CheckoutSession create transaction.
// Therefore a phone that has no BNPL customer must durably close the already
// created attempt instead of leaving an unreachable open session behind.
// Transient Identity failures never call this helper and remain retryable with
// the same idempotency key.
func (uc *UseCase) persistExternalUnavailable(
	ctx context.Context,
	token model.CheckoutSessionToken,
) (model.CheckoutSession, error) {
	var (
		unavailable model.CheckoutSession
		expired     bool
	)

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			sessionQ := uc.checkoutSession.Tx(tx)

			locked, lockErr := uc.lockExternalSession(ctx, tx, token)
			if lockErr != nil {
				return lockErr
			}

			now := uc.clock.Now().UTC()

			if locked.Status == model.UnavailableCheckoutSessionStatus {
				unavailable = locked
				return nil
			}

			if locked.Status != model.OpenCheckoutSessionStatus || locked.ContractID != nil {
				return checkoutSessionNotContinuableError(
					fmt.Errorf(
						"checkoutsessionuc.UseCase.persistExternalUnavailable: checkout session status is %q",
						locked.Status,
					),
				)
			}

			// A losing Identity NOT_FOUND must not overwrite a concurrent caller
			// that already resolved/bound the checkout.
			if locked.CustomerID != nil || (locked.DecisionID != nil && !locked.DecisionID.IsZero()) {
				return cerr.Conflict(
					fmt.Errorf(
						"checkoutsessionuc.UseCase.persistExternalUnavailable: checkout preparation advanced concurrently",
					),
				)
			}

			// Hard session deadline wins over a late Identity NOT_FOUND.
			if locked.IsExpiredAt(now) {
				transitioned, statusErr := uc.transitionToExpired(ctx, tx, locked, now)
				if statusErr != nil {
					return statusErr
				}

				unavailable = transitioned
				expired = true
				return nil
			}

			if transitionErr := locked.Status.ValidateTransitionTo(model.UnavailableCheckoutSessionStatus); transitionErr != nil {
				return cerr.InternalError(
					fmt.Errorf("checkoutsessionuc.UseCase.persistExternalUnavailable: %w", transitionErr),
				)
			}

			updated, statusErr := sessionQ.UpdateStatus(
				ctx, locked.ID, model.UnavailableCheckoutSessionStatus, now,
			)
			if statusErr != nil {
				return statusErr
			}

			if enqueueErr := uc.lifecycle.Enqueue(
				ctx, tx, updated, checkoutlifecycle.EventTypeUnavailable, now,
			); enqueueErr != nil {
				return enqueueErr
			}

			unavailable = updated
			return nil
		})
	})
	if err != nil {
		return model.CheckoutSession{}, err
	}

	if expired {
		return unavailable, checkoutSessionExpiredError()
	}

	return unavailable, nil
}

// persistExternalExpired marks an open external checkout expired under lock.
func (uc *UseCase) persistExternalExpired(
	ctx context.Context,
	token model.CheckoutSessionToken,
) (model.CheckoutSession, error) {
	var expired model.CheckoutSession

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			locked, lockedExpired, err := uc.lockExternalContinuable(ctx, tx, token)
			if err != nil {
				return err
			}

			if lockedExpired {
				expired = locked

				return nil
			}

			now := uc.clock.Now().UTC()
			updated, statusErr := uc.transitionToExpired(ctx, tx, locked, now)
			if statusErr != nil {
				return statusErr
			}

			expired = updated

			return nil
		})
	})
	if err != nil {
		return model.CheckoutSession{}, err
	}

	return expired, nil
}
