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
	financialPartyTypePlatform            = "platform"
	// financialPlatformPayeeID is empty by Financial Core contract for platform
	// checkout payees. Merchant economics stay on PrepareCollectionObligation /
	// RecordMerchantEntitlement — not on CreateCheckoutPayment payee.
	financialPlatformPayeeID     = ""
	maxFinancialIdentifierLength = 256
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

		if contractRow.Status != model.ActiveContractStatus && !(contractRow.Status == model.CompletedContractStatus && model.IsChargeOnlyPayment(attempt.Items)) {
			return contractNotActiveError()
		}

		if contractRow.FinancialMerchantEntitlementID == nil || strings.TrimSpace(*contractRow.FinancialMerchantEntitlementID) == "" {
			return financialReceivableNotReadyError()
		}

		customerProfileQ := uc.customerProfile.Conn(conn)
		customer, loadErr = customerProfileQ.GetByExternalCustomerID(ctx, input.ExternalCustomerID)
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

	obligationID := ""
	if attempt.FinancialCollectionObligationID != nil {
		obligationID = strings.TrimSpace(*attempt.FinancialCollectionObligationID)
	}

	if obligationID == "" {
		prepared, prepareErr := uc.checkoutPaymentClient.PrepareCollectionObligation(ctx, extcheckoutpayment.PrepareCollectionObligationInput{
			SourceType:            financialCorePaymentAttemptSourceType,
			SourceID:              attempt.ID.String(),
			SourceVersion:         financialCoreSourceVersionPtr(),
			MerchantID:            merchant.MerchantID.String(),
			CustomerID:            customer.ExternalCustomerID.String(),
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
			return model.PaymentAttempt{}, financialCoreInstallmentPaymentUnavailableError(errors.New("financial core collection amount or currency differs from immutable payment aggregate"))
		}

		obligationID = strings.TrimSpace(prepared.CollectionObligationID)
		if obligationID == "" || len(obligationID) > maxFinancialIdentifierLength {
			return model.PaymentAttempt{}, financialCoreInstallmentPaymentUnavailableError(errors.New("financial core returned invalid collection obligation id"))
		}

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

	created, err := uc.checkoutPaymentClient.CreateCheckoutPayment(ctx, extcheckoutpayment.CreateCheckoutPaymentInput{
		IdempotencyKey:         attempt.IdempotencyKey,
		SourceType:             financialCorePaymentAttemptSourceType,
		SourceID:               attempt.ID.String(),
		SourceVersion:          financialCoreSourceVersionPtr(),
		PayerType:              financialPartyTypeCustomer,
		PayerID:                customer.ExternalCustomerID.String(),
		PayeeType:              financialPartyTypePlatform,
		PayeeID:                financialPlatformPayeeID,
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
		return model.PaymentAttempt{}, err
	}

	// Validate before MarkSubmitted. An incomplete/malformed FC response is not
	// proof that FC failed to commit — leave the attempt in created for
	// reconciliation and preserve the original idempotency identity.
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
		return model.PaymentAttempt{}, mapInstallmentCheckoutPaymentValidationError(err)
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
			return markErr
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
