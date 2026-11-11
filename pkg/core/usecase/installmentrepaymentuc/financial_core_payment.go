package installmentrepaymentuc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	extcheckoutpayment "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

const (
	financialCorePaymentAttemptSourceType = "bnpl_payment_attempt"
	financialPartyTypeCustomer            = "customer"
	maxFinancialIdentifierLength          = 256
)

func financialCoreSourceVersionPtr() *int64 {
	v := int64(1)
	return &v
}

// ensureFinancialCoreInstallmentPayment creates or recovers the FC collection
// obligation and checkout session for installments after the activation payment.
// Network calls intentionally run outside the database transaction that created
// the local PaymentAttempt.
func (uc *UseCase) ensureFinancialCoreInstallmentPayment(
	ctx context.Context,
	attempt model.PaymentAttempt,
	input CreatePaymentAttemptInput,
) (model.PaymentAttempt, error) {
	const op = "installmentrepaymentuc.UseCase.ensureFinancialCoreInstallmentPayment"

	if attempt.State == model.SubmittedPaymentAttemptState || attempt.State == model.SucceededPaymentAttemptState {
		return attempt, nil
	}

	if attempt.State != model.CreatedPaymentAttemptState {
		return model.PaymentAttempt{}, cerr.Conflict(
			fmt.Errorf("%s: payment attempt state %q cannot create a Financial Core session", op, attempt.State),
			map[string]any{"payment_attempt_id": attempt.ID, "state": attempt.State},
		)
	}

	// Existing attempts created before all request metadata was persisted are
	// enriched idempotently from the current authenticated request before any FC call.
	if attempt.TenantID == nil || attempt.ContextID == nil || attempt.SuccessReturnURL == nil ||
		attempt.FailureReturnURL == nil || attempt.CancelReturnURL == nil {
		now := uc.clock.Now().UTC()
		var bound model.PaymentAttempt
		err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
			return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
				var bindErr error
				paymentAttemptQ := uc.paymentAttempt.Tx(tx)
				bound, bindErr = paymentAttemptQ.BindFinancialRequestContextIfAbsent(
					ctx,
					attempt.ID,
					input.TenantID.String(),
					input.ContextID.String(),
					optionalTrimmedString(input.ProviderCode),
					uc.paymentReturnURL,
					uc.paymentReturnURL,
					uc.paymentReturnURL,
					now,
				)
				return bindErr
			})
		})
		if err != nil {
			return model.PaymentAttempt{}, err
		}

		bound.Items = attempt.Items
		attempt = bound
	}

	var (
		contractRow    model.Contract
		customer       model.CustomerProfile
		merchant       model.MerchantProfile
		installmentRow model.Installment
	)
	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		var loadErr error
		contractQ := uc.contract.Conn(conn)
		contractRow, loadErr = contractQ.GetByID(ctx, attempt.ContractID)
		if loadErr != nil {
			return loadErr
		}

		if contractRow.Status != model.ActiveContractStatus {
			return contractNotActiveError()
		}

		if contractRow.FinancialMerchantEntitlementID == nil || strings.TrimSpace(*contractRow.FinancialMerchantEntitlementID) == "" {
			return financialReceivableNotReadyError()
		}

		customerProfileQ := uc.customerProfile.Conn(conn)
		customer, loadErr = customerProfileQ.GetByExternalSubjectID(ctx, input.ExternalSubjectID)
		if loadErr != nil {
			return loadErr
		}

		if customer.ID != contractRow.CustomerID {
			return contractDoesNotBelongToCustomerError(errors.New("contract does not belong to customer profile"))
		}

		merchantProfileQ := uc.merchantProfile.Conn(conn)
		merchant, loadErr = merchantProfileQ.GetByID(ctx, contractRow.MerchantProfileID)
		if loadErr != nil {
			return loadErr
		}

		installmentQ := uc.installment.Conn(conn)
		installmentRow, loadErr = installmentQ.GetByID(ctx, attempt.InstallmentID)
		if loadErr != nil {
			return loadErr
		}

		if !installmentBelongsToContract(installmentRow, attempt.ContractID) {
			return installmentDoesNotBelongToContractError()
		}

		snapshotAmount, ok := model.PayableAmountFromItems(attempt.Items)
		if !ok || snapshotAmount != attempt.AmountMinor {
			return cerr.InternalError(fmt.Errorf("%s: inconsistent aggregate payment snapshot", op))
		}

		return nil
	})
	if err != nil {
		return model.PaymentAttempt{}, err
	}

	amountMinor, err := model.AmountMinorInt64(attempt.AmountMinor)
	if err != nil {
		return model.PaymentAttempt{}, cerr.InternalError(fmt.Errorf("%s: amount: %w", op, err))
	}

	// Always Prepare before CreateCheckoutPayment so payment_recipient_* is taken
	// from Financial Core. Prepare is idempotent for the same source; a stored
	// obligation id must match the Prepare result.
	prepared, prepareErr := uc.checkoutPaymentClient.PrepareCollectionObligation(ctx, extcheckoutpayment.PrepareCollectionObligationInput{
		SourceType:            financialCorePaymentAttemptSourceType,
		SourceID:              attempt.ID.String(),
		SourceVersion:         financialCoreSourceVersionPtr(),
		ExternalMerchantID: merchant.ExternalMerchantID.String(),
		ExternalCustomerID:    customer.ExternalCustomerID.String(),
		Currency:              string(attempt.Currency),
		AmountMinor:           amountMinor,
		Application:           extcheckoutpayment.ExistingReceivableCollectionApplication,
		MerchantEntitlementID: strings.TrimSpace(*contractRow.FinancialMerchantEntitlementID),
		Description:           "BNPL installment repayment",
		DueAt:                 installmentRow.DueAt,
	})
	if prepareErr != nil {
		return model.PaymentAttempt{}, prepareErr
	}

	if prepared.OutstandingAmountMinor != amountMinor || prepared.Currency != string(attempt.Currency) {
		return model.PaymentAttempt{}, financialCoreInstallmentPaymentUnavailableError(
			errors.New("financial core collection amount or currency differs from immutable payment aggregate"),
		)
	}

	obligationID := strings.TrimSpace(prepared.CollectionObligationID)
	if obligationID == "" || len(obligationID) > maxFinancialIdentifierLength {
		return model.PaymentAttempt{}, financialCoreInstallmentPaymentUnavailableError(
			errors.New("financial core returned invalid collection obligation id"),
		)
	}

	payeeType, payeeID, err := extcheckoutpayment.CheckoutPayeeFromPrepare(prepared)
	if err != nil {
		return model.PaymentAttempt{}, financialCoreInstallmentPaymentUnavailableError(err)
	}

	existingObligationID := ""
	if attempt.FinancialCollectionObligationID != nil {
		existingObligationID = strings.TrimSpace(*attempt.FinancialCollectionObligationID)
	}

	if existingObligationID != "" && existingObligationID != obligationID {
		return model.PaymentAttempt{}, financialCoreInstallmentPaymentUnavailableError(
			errors.New("financial core returned conflicting collection obligation id"),
		)
	}

	if existingObligationID == "" {
		now := uc.clock.Now().UTC()
		err = uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
			return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
				paymentAttemptQ := uc.paymentAttempt.Tx(tx)
				bound, bindErr := paymentAttemptQ.BindCollectionObligationIfAbsent(ctx, attempt.ID, obligationID, now)
				if bindErr == nil {
					bound.Items = attempt.Items
					attempt = bound
				}

				return bindErr
			})
		})
		if err != nil {
			return model.PaymentAttempt{}, err
		}
	}

	providerCode := ""
	if attempt.ProviderCode != nil {
		providerCode = strings.TrimSpace(*attempt.ProviderCode)
	}

	if attempt.SuccessReturnURL == nil || attempt.FailureReturnURL == nil || attempt.CancelReturnURL == nil {
		return model.PaymentAttempt{}, cerr.InternalError(fmt.Errorf("%s: financial request context is incomplete", op))
	}

	now := uc.clock.Now().UTC()
	requestedExpiresAt, err := uc.requestedCheckoutPaymentExpiresAt(attempt, now)
	if err != nil {
		return model.PaymentAttempt{}, err
	}

	// Reserve one CreateCheckoutPayment attempt against the per-idempotency-key
	// budget before any network call. A fourth reservation is refused and, when
	// the row is still only locally created with three completed invocations,
	// finalized as canceled without inventing FC financial resolution.
	syncBudgetExhausted := false
	err = uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			paymentAttemptQ := uc.paymentAttempt.Tx(tx)
			locked, lockErr := paymentAttemptQ.LockByID(ctx, attempt.ID)
			if lockErr != nil {
				return lockErr
			}

			if locked.IsLocalSessionCreationRetryExhausted() {
				locked.Items = attempt.Items
				attempt = locked
				syncBudgetExhausted = true
				return nil
			}

			if !locked.CanReserveFinancialSessionCreate() {
				return cerr.Conflict(
					fmt.Errorf("%s: payment attempt state %q cannot create a Financial Core session", op, locked.State),
					map[string]any{"payment_attempt_id": locked.ID, "state": locked.State},
				)
			}

			if locked.FinancialSessionCreateLeaseActive(now) {
				return financialSessionCreateInProgressError()
			}

			if locked.FinancialSessionCreateAttemptCount >= model.MaxFinancialSessionCreateAttempts {
				outboxQ := uc.outbox.Tx(tx)
				canceled, cancelErr := uc.cancelSessionCreationRetryExhaustedInTx(
					ctx,
					paymentAttemptQ,
					outboxQ,
					locked,
					now,
				)
				if cancelErr != nil {
					return cancelErr
				}

				canceled.Items = attempt.Items
				attempt = canceled
				syncBudgetExhausted = true
				return nil
			}

			reserved, reserveErr := paymentAttemptQ.IncrementFinancialSessionCreateAttemptCount(
				ctx,
				locked.ID,
				model.MaxFinancialSessionCreateAttempts,
				now,
			)
			if reserveErr != nil {
				return reserveErr
			}

			reserved.Items = attempt.Items
			attempt = reserved
			return nil
		})
	})
	if err != nil {
		return model.PaymentAttempt{}, err
	}

	if syncBudgetExhausted {
		return attempt, financialSessionCreateRetryBudgetExhaustedError()
	}

	// Idempotent recovery: reuse attempt.IdempotencyKey so a prior FC success
	// that BNPL never observed returns the same immutable PaymentIntent,
	// PaymentSession, and PaymentLink. Do not mint a new key to rediscover URL.
	// GetPaymentStatus cannot supply PaymentURL.
	created, err := uc.checkoutPaymentClient.CreateCheckoutPayment(ctx, extcheckoutpayment.CreateCheckoutPaymentInput{
		IdempotencyKey:         attempt.IdempotencyKey,
		SourceType:             financialCorePaymentAttemptSourceType,
		SourceID:               attempt.ID.String(),
		SourceVersion:          financialCoreSourceVersionPtr(),
		PayerType:              financialPartyTypeCustomer,
		PayerID:                customer.ExternalCustomerID.String(),
		PayeeType:              payeeType,
		PayeeID:                payeeID,
		AmountMinor:            amountMinor,
		Currency:               string(attempt.Currency),
		ProviderCode:           providerCode,
		CollectionObligationID: obligationID,
		SuccessReturnURL:       strings.TrimSpace(*attempt.SuccessReturnURL),
		FailureReturnURL:       strings.TrimSpace(*attempt.FailureReturnURL),
		CancelReturnURL:        strings.TrimSpace(*attempt.CancelReturnURL),
		Description:            "BNPL installment repayment",
		ExpiresAt:              requestedExpiresAt,
	})
	if err != nil {
		locallyCanceled, markErr := uc.finalizeFailedSessionCreateAttempt(ctx, attempt, now)
		if markErr != nil {
			return model.PaymentAttempt{}, errors.Join(err, markErr)
		}
		if locallyCanceled {
			return model.PaymentAttempt{}, financialSessionCreateRetryBudgetExhaustedError()
		}

		return model.PaymentAttempt{}, err
	}

	// Validate before MarkSubmitted. An incomplete/malformed FC response is not
	// proof that FC failed to commit — after the final permitted sync attempt,
	// cancel locally; otherwise leave created for customer retry.
	validationNow := uc.clock.Now().UTC()
	if err := extcheckoutpayment.ValidateRequiresActionCreateResult(
		created,
		extcheckoutpayment.RequiresActionCreateResultCheck{
			ExpectedAmountMinor: amountMinor,
			ExpectedCurrency:    string(attempt.Currency),
			Now:                 validationNow,
			MaxIdentifierLength: maxFinancialIdentifierLength,
		},
	); err != nil {
		mapped := mapInstallmentCheckoutPaymentValidationError(err)
		locallyCanceled, markErr := uc.finalizeFailedSessionCreateAttempt(ctx, attempt, validationNow)
		if markErr != nil {
			return model.PaymentAttempt{}, errors.Join(mapped, markErr)
		}
		if locallyCanceled {
			return model.PaymentAttempt{}, financialSessionCreateRetryBudgetExhaustedError()
		}

		return model.PaymentAttempt{}, mapped
	}

	intentID := strings.TrimSpace(created.PaymentIntentID)
	sessionID := strings.TrimSpace(created.PaymentSessionID)
	paymentURL := strings.TrimSpace(created.PaymentLink)
	// Authoritative FC session expiry (distinct from requested CreatedAt+TTL).
	expiresAt := created.ExpiresAt.UTC()
	extcheckoutpayment.WarnIfExpiresAtLaterThanRequested(
		uc.logger,
		requestedExpiresAt,
		expiresAt,
		"payment_attempt_id", attempt.ID.String(),
		"source_id", attempt.ID.String(),
		"payment_operation_type", string(attempt.Purpose),
	)

	submittedAt := validationNow
	var submitted model.PaymentAttempt
	err = uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			var markErr error
			paymentAttemptQ := uc.paymentAttempt.Tx(tx)
			submitted, markErr = paymentAttemptQ.MarkSubmitted(
				ctx,
				attempt.ID,
				intentID,
				sessionID,
				&paymentURL,
				&expiresAt,
				submittedAt,
				submittedAt,
			)
			if markErr == nil {
				return nil
			}

			// A reconciliation job may have moved the attempt out of created
			// while this CreateCheckoutPayment call was in flight. A valid FC
			// session for the same immutable operation must still be recorded
			// when the local row is only reconciliation_required for uncertainty
			// and does not already hold a conflicting intent.
			if !cerr.IsCode(markErr, cerr.CodeConflict) {
				return markErr
			}

			locked, lockErr := paymentAttemptQ.LockByID(ctx, attempt.ID)
			if lockErr != nil {
				return lockErr
			}

			if locked.FinancialPaymentIntentID != nil &&
				strings.TrimSpace(*locked.FinancialPaymentIntentID) != "" &&
				strings.TrimSpace(*locked.FinancialPaymentIntentID) != intentID {
				return markErr
			}

			switch locked.State {
			case model.ReconciliationRequiredPaymentAttemptState:
				if locked.ReconciliationReason == nil ||
					!locked.ReconciliationReason.AllowsRecoveredPaymentSession() {
					return markErr
				}

				submitted, markErr = paymentAttemptQ.MarkSubmittedFromReconciliationRequired(
					ctx,
					locked.ID,
					intentID,
					sessionID,
					&paymentURL,
					&expiresAt,
					submittedAt,
					submittedAt,
					submittedAt,
				)
				return markErr

			case model.ExpiredPaymentAttemptState:
				// Late verified FC response after local expiration. A still-usable
				// session may recover to submitted; an already-elapsed FC session
				// records identity only and stays expired (session creation remains closed).
				if !expiresAt.After(submittedAt) {
					_, enrichErr := paymentAttemptQ.EnrichFinancialPaymentSessionIfAbsent(
						ctx,
						locked.ID,
						intentID,
						sessionID,
						nil,
						paymentURL,
						expiresAt,
						submittedAt,
					)
					if enrichErr != nil {
						return enrichErr
					}

					locked.Items = attempt.Items
					submitted = locked
					return nil
				}

				if err := locked.State.ValidateTransitionTo(model.SubmittedPaymentAttemptState); err != nil {
					return markErr
				}

				submitted, markErr = paymentAttemptQ.MarkSubmittedFromExpired(
					ctx,
					locked.ID,
					intentID,
					sessionID,
					&paymentURL,
					&expiresAt,
					submittedAt,
					submittedAt,
				)
				return markErr

			default:
				return markErr
			}
		})
	})
	if err != nil {
		// Persistence failure after a valid FC response remains ambiguous: do not
		// invent failure or a replacement PaymentAttempt.
		return model.PaymentAttempt{}, err
	}

	submitted.Items = attempt.Items
	return submitted, nil
}

func mapInstallmentCheckoutPaymentValidationError(err error) error {
	if errors.Is(err, extcheckoutpayment.ErrAmountCurrencyMismatch) {
		return financialCoreInstallmentPaymentConflictError(err)
	}

	return financialCoreInstallmentPaymentUnavailableError(err)
}

// finalizeFailedSessionCreateAttempt runs after an owned synchronous
// CreateCheckoutPayment call fails. A stale caller must never release another
// caller's lease or cancel its in-flight third session-creation call.
// The returned bool reports only a durably committed local cancellation.
func (uc *UseCase) finalizeFailedSessionCreateAttempt(
	ctx context.Context,
	attempt model.PaymentAttempt,
	now time.Time,
) (bool, error) {
	if attempt.State != model.CreatedPaymentAttemptState {
		return false, nil
	}

	finalizeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	var locallyCanceled bool
	err := uc.pool.Conn(finalizeCtx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			paymentAttemptQ := uc.paymentAttempt.Tx(tx)
			outboxQ := uc.outbox.Tx(tx)
			locked, lockErr := paymentAttemptQ.LockByID(ctx, attempt.ID)
			if lockErr != nil {
				return lockErr
			}
			if locked.State != model.CreatedPaymentAttemptState {
				return nil
			}

			// Count can already be three because a later request owns the
			// third reservation. Ownership must be checked before finalizing.
			if attempt.FinancialSessionCreateLeaseToken == nil ||
				locked.FinancialSessionCreateLeaseToken == nil ||
				*attempt.FinancialSessionCreateLeaseToken != *locked.FinancialSessionCreateLeaseToken {
				return nil
			}

			if locked.FinancialSessionCreateAttemptCount < model.MaxFinancialSessionCreateAttempts {
				_, clearErr := paymentAttemptQ.ClearFinancialSessionCreateLease(
					ctx,
					locked.ID,
					*attempt.FinancialSessionCreateLeaseToken,
					now.UTC(),
				)
				if cerr.IsCode(clearErr, cerr.CodeConflict) {
					return nil
				}
				return clearErr
			}

			_, cancelErr := uc.cancelSessionCreationRetryExhaustedInTx(
				ctx, paymentAttemptQ, outboxQ, locked, now.UTC(),
			)
			if cancelErr == nil {
				locallyCanceled = true
			}
			return cancelErr
		})
	})
	if err != nil {
		return false, err
	}
	return locallyCanceled, nil
}

func (uc *UseCase) cancelSessionCreationRetryExhaustedInTx(
	ctx context.Context,
	paymentAttemptQ repo.PaymentAttemptTxQueryer,
	outboxQ repo.OutboxTxQueryer,
	locked model.PaymentAttempt,
	now time.Time,
) (model.PaymentAttempt, error) {
	if locked.IsLocalSessionCreationRetryExhausted() {
		return locked, nil
	}

	if err := locked.State.ValidateTransitionTo(model.CanceledPaymentAttemptState); err != nil {
		return model.PaymentAttempt{}, err
	}

	canceled, err := paymentAttemptQ.CancelSessionCreationRetryExhausted(
		ctx,
		locked.ID,
		locked.State,
		now,
		now,
	)
	if err != nil {
		return model.PaymentAttempt{}, err
	}

	if err := uc.insertSessionCreationRetryExhaustedAuditOutbox(
		ctx,
		outboxQ,
		locked,
		canceled,
		now,
	); err != nil {
		return model.PaymentAttempt{}, err
	}

	return canceled, nil
}

// requestedCheckoutPaymentExpiresAt returns the immutable CreateCheckoutPayment
// expires_at for attempt: CreatedAt + paymentSessionTTL. Retries must reuse this
// value; it is never recomputed from clock.Now. If the window has already ended,
// FC is not called with an extended deadline.
func (uc *UseCase) requestedCheckoutPaymentExpiresAt(
	attempt model.PaymentAttempt,
	now time.Time,
) (time.Time, error) {
	const op = "installmentrepaymentuc.UseCase.requestedCheckoutPaymentExpiresAt"

	if attempt.CreatedAt.IsZero() {
		return time.Time{}, cerr.InternalError(fmt.Errorf("%s: payment attempt created_at is required", op))
	}

	ttl := uc.paymentSessionTTL
	if ttl <= 0 {
		ttl = DefaultPaymentSessionTTL
	}

	expiresAt := attempt.CreatedAt.UTC().Add(ttl)
	if !expiresAt.After(now.UTC()) {
		return time.Time{}, installmentPaymentSessionRequestExpiredError(
			fmt.Errorf(
				"%s: requested payment session window ended at %s",
				op,
				expiresAt.UTC().Format(time.RFC3339Nano),
			),
		)
	}

	return expiresAt, nil
}

func optionalTrimmedString(raw string) *string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil
	}

	return &value
}
