package contractuc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	extcheckoutpayment "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

func contractHasFinancialCheckoutPaymentSession(contractRow model.Contract) bool {
	return model.ContractHasFinancialCheckoutPaymentSession(contractRow)
}

func paymentAttemptHasFinancialCheckoutSession(attempt model.PaymentAttempt) bool {
	return model.PaymentAttemptHasFinancialCheckoutSession(attempt)
}

func checkoutContractSessionMatchesAttempt(contractRow model.Contract, attempt model.PaymentAttempt) bool {
	return model.CheckoutContractSessionMatchesAttempt(contractRow, attempt)
}

func checkoutPaymentOutputFromContract(contractRow model.Contract) CheckoutPaymentOutput {
	output := CheckoutPaymentOutput{
		ReusedIdempotentResult: contractRow.FinancialPaymentReusedIdempotentResult,
	}

	if contractRow.FinancialInvoiceID != nil {
		output.InvoiceID = *contractRow.FinancialInvoiceID
	}

	if contractRow.FinancialPaymentIntentID != nil {
		output.PaymentIntentID = *contractRow.FinancialPaymentIntentID
	}

	if contractRow.FinancialPaymentAttemptID != nil {
		output.PaymentAttemptID = *contractRow.FinancialPaymentAttemptID
	}

	if contractRow.FinancialPaymentSessionID != nil {
		output.PaymentSessionID = *contractRow.FinancialPaymentSessionID
	}

	if contractRow.FinancialPaymentLink != nil {
		output.PaymentLink = *contractRow.FinancialPaymentLink
	}

	if contractRow.FinancialPaymentExpiresAt != nil {
		output.ExpiresAt = contractRow.FinancialPaymentExpiresAt.UTC().Format(time.RFC3339)
	}

	return output
}

func checkoutPaymentOutputFromResult(
	result extcheckoutpayment.CreateCheckoutPaymentResult,
	fallbackExpiresAt time.Time,
) CheckoutPaymentOutput {
	expiresAt := fallbackExpiresAt.UTC()
	if !result.ExpiresAt.IsZero() {
		expiresAt = result.ExpiresAt.UTC()
	}

	return CheckoutPaymentOutput{
		InvoiceID:              strings.TrimSpace(result.InvoiceID),
		PaymentIntentID:        strings.TrimSpace(result.PaymentIntentID),
		PaymentAttemptID:       strings.TrimSpace(result.PaymentAttemptID),
		PaymentSessionID:       strings.TrimSpace(result.PaymentSessionID),
		PaymentLink:            strings.TrimSpace(result.PaymentLink),
		ExpiresAt:              expiresAt.Format(time.RFC3339),
		ReusedIdempotentResult: result.ReusedIdempotentResult,
	}
}

func checkoutPaymentIdempotencyKeyPrefix(contractID model.ContractID) string {
	return fmt.Sprintf("bnpl:contract:%s:checkout-payment", contractID)
}

func checkoutPaymentIdempotencyKey(contractID model.ContractID, retryIDs ...uuid.UUID) string {
	prefix := checkoutPaymentIdempotencyKeyPrefix(contractID)
	if len(retryIDs) == 0 || retryIDs[0] == uuid.Nil {
		return prefix
	}

	return fmt.Sprintf("%s:%s", prefix, retryIDs[0])
}

func isCheckoutPaymentIdempotencyKey(contractID model.ContractID, value string) bool {
	prefix := checkoutPaymentIdempotencyKeyPrefix(contractID)
	if value == prefix {
		return true
	}

	return isCheckoutPaymentRetryIdempotencyKey(contractID, value)
}

func isCheckoutPaymentRetryIdempotencyKey(contractID model.ContractID, value string) bool {
	prefix := checkoutPaymentIdempotencyKeyPrefix(contractID)
	suffix := strings.TrimPrefix(value, prefix+":")
	if suffix == value {
		return false
	}

	retryID, err := uuid.Parse(suffix)
	return err == nil && retryID != uuid.Nil
}

func checkoutPaymentRequestFingerprint(input CreateContractInput, contractRow model.Contract) string {
	// Local BNPL fingerprint for PaymentAttempt durability. Financial Core owns
	// its own request fingerprint and must not receive this value on the wire.
	raw := fmt.Sprintf(
		"contract:%s:amount:%d:currency:%s:customer:%s:merchant:%s:expires:%s:provider:%s",
		contractRow.ID,
		contractRow.PayableNowAmountMinor,
		contractRow.Currency,
		contractRow.CustomerID,
		contractRow.MerchantProfileID,
		contractRow.ExpiresAt.UTC().Format(time.RFC3339Nano),
		strings.TrimSpace(input.ProviderCode),
	)

	sum := sha256.Sum256([]byte(raw))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func financialSourceVersion() *int64 {
	v := int64(1)
	return &v
}

func (uc *UseCase) buildCheckoutPaymentInputFromAttempt(
	contractRow model.Contract,
	attempt model.PaymentAttempt,
	externalCustomerID model.ExternalCustomerID,
) (extcheckoutpayment.CreateCheckoutPaymentInput, error) {
	const op = "contractuc.UseCase.buildCheckoutPaymentInputFromAttempt"

	if err := model.ValidatePositiveAmountMinor(contractRow.PayableNowAmountMinor); err != nil {
		return extcheckoutpayment.CreateCheckoutPaymentInput{}, cerr.InternalError(fmt.Errorf("%s: checkout payable amount: %w", op, err))
	}

	if attempt.SuccessReturnURL == nil || attempt.FailureReturnURL == nil || attempt.CancelReturnURL == nil {
		return extcheckoutpayment.CreateCheckoutPaymentInput{}, cerr.InternalError(fmt.Errorf("%s: checkout payment attempt return url is incomplete", op))
	}

	collectionObligationID := optionalStringValue(attempt.FinancialCollectionObligationID)
	if collectionObligationID == "" {
		return extcheckoutpayment.CreateCheckoutPaymentInput{}, cerr.InternalError(
			fmt.Errorf("%s: collection obligation target is required before checkout payment", op),
		)
	}

	if strings.TrimSpace(externalCustomerID.String()) == "" {
		return extcheckoutpayment.CreateCheckoutPaymentInput{}, cerr.InternalError(
			fmt.Errorf("%s: trusted customer reference id is required", op),
		)
	}

	providerCode := ""
	if attempt.ProviderCode != nil {
		providerCode = strings.TrimSpace(*attempt.ProviderCode)
	}

	amountMinor, err := model.AmountMinorInt64(contractRow.PayableNowAmountMinor)
	if err != nil {
		return extcheckoutpayment.CreateCheckoutPaymentInput{}, cerr.InternalError(fmt.Errorf("%s: checkout payable amount: %w", op, err))
	}

	return extcheckoutpayment.CreateCheckoutPaymentInput{
		IdempotencyKey: attempt.IdempotencyKey,

		SourceType:    financialSourceType,
		SourceID:      contractRow.ID.String(),
		SourceVersion: financialSourceVersion(),

		PayerType: financialPayerType,
		PayerID:   externalCustomerID.String(),
		PayeeType: financialPayeeType,
		PayeeID:   financialPlatformPayeeID,

		AmountMinor: amountMinor,
		Currency:    string(contractRow.Currency),

		ProviderCode:           providerCode,
		CollectionObligationID: collectionObligationID,

		SuccessReturnURL: strings.TrimSpace(*attempt.SuccessReturnURL),
		FailureReturnURL: strings.TrimSpace(*attempt.FailureReturnURL),
		CancelReturnURL:  strings.TrimSpace(*attempt.CancelReturnURL),

		Description: fmt.Sprintf("BNPL checkout contract %s", contractRow.ID),
		ExpiresAt:   contractRow.ExpiresAt.UTC(),
	}, nil
}

func checkoutActivationCollectionAmount(attempt model.PaymentAttempt) (uint64, error) {
	const op = "contractuc.checkoutActivationCollectionAmount"

	var total uint64
	for _, item := range attempt.Items {
		if item.Effect != model.DebitPaymentAttemptItemEffect {
			return 0, cerr.InternalError(fmt.Errorf("%s: checkout activation does not support credit items", op))
		}

		switch item.ItemType {
		case model.PrincipalPaymentAttemptItemType, model.UpfrontPaymentAttemptItemType:
			var err error
			total, err = model.AddAmountMinor(total, item.AmountMinor)
			if err != nil {
				return 0, cerr.InternalError(fmt.Errorf("%s: sum activation cash: %w", op, err))
			}

		case model.ChargePaymentAttemptItemType:
			return 0, cerr.InternalError(fmt.Errorf("%s: first-payment FC flow supports only upfront + first principal", op))

		default:
			return 0, cerr.InternalError(fmt.Errorf("%s: unsupported checkout payment attempt item type %q", op, item.ItemType))
		}
	}

	if total == 0 {
		return 0, cerr.InternalError(fmt.Errorf("%s: activation collection amount is zero", op))
	}

	return total, nil
}

func (uc *UseCase) loadCheckoutFinancialParties(
	ctx context.Context,
	contractRow model.Contract,
) (model.CustomerProfile, model.MerchantProfile, error) {
	const op = "contractuc.UseCase.loadCheckoutFinancialParties"

	var customer model.CustomerProfile
	var merchant model.MerchantProfile

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		var err error
		customerProfileQ := uc.customerProfile.Conn(conn)
		customer, err = customerProfileQ.GetByID(ctx, contractRow.CustomerID)
		if err != nil {
			return err
		}

		merchantProfileQ := uc.merchantProfile.Conn(conn)
		merchant, err = merchantProfileQ.GetByID(ctx, contractRow.MerchantProfileID)
		return err
	})
	if err != nil {
		return model.CustomerProfile{}, model.MerchantProfile{}, err
	}

	if customer.ExternalCustomerID.IsZero() || merchant.MerchantID.IsZero() {
		return model.CustomerProfile{}, model.MerchantProfile{}, cerr.InternalError(
			fmt.Errorf("%s: checkout financial party identity is missing", op),
		)
	}

	return customer, merchant, nil
}

func (uc *UseCase) ensureCheckoutCollectionObligation(
	ctx context.Context,
	contractRow model.Contract,
	attempt model.PaymentAttempt,
	customer model.CustomerProfile,
	merchant model.MerchantProfile,
) (model.PaymentAttempt, error) {
	const op = "contractuc.UseCase.ensureCheckoutCollectionObligation"

	if attempt.FinancialCollectionObligationID != nil && strings.TrimSpace(*attempt.FinancialCollectionObligationID) != "" {
		return attempt, nil
	}

	amount, err := checkoutActivationCollectionAmount(attempt)
	if err != nil {
		return model.PaymentAttempt{}, err
	}

	if amount != contractRow.PayableNowAmountMinor {
		return model.PaymentAttempt{}, cerr.InternalError(fmt.Errorf(
			"%s: checkout payable amount must equal upfront plus first principal for FC activation",
			op,
		))
	}

	amountMinor, err := model.AmountMinorInt64(amount)
	if err != nil {
		return model.PaymentAttempt{}, cerr.InternalError(fmt.Errorf("%s: checkout activation amount: %w", op, err))
	}

	result, err := uc.checkoutPaymentClient.PrepareCollectionObligation(ctx, extcheckoutpayment.PrepareCollectionObligationInput{
		SourceType:    financialSourceType,
		SourceID:      contractRow.ID.String(),
		SourceVersion: financialSourceVersion(),
		MerchantID:    merchant.MerchantID.String(),
		CustomerID:    customer.ExternalCustomerID.String(),
		Currency:      string(contractRow.Currency),
		AmountMinor:   amountMinor,
		Application:   extcheckoutpayment.PreRecognitionCashCollectionApplication,
		Description:   fmt.Sprintf("BNPL checkout contract %s activation collection", contractRow.ID),
		DueAt:         contractRow.ExpiresAt.UTC(),
	})
	if err != nil {
		return model.PaymentAttempt{}, err
	}

	obligationID := strings.TrimSpace(result.CollectionObligationID)
	if obligationID == "" || len(obligationID) > maxFinancialIdentifierLength ||
		result.OutstandingAmountMinor != amountMinor || result.Currency != string(contractRow.Currency) {
		return model.PaymentAttempt{}, financialCoreCheckoutPaymentUnavailableError(
			fmt.Errorf("%s: financial core returned invalid collection obligation", op),
		)
	}

	var saved model.PaymentAttempt
	now := uc.clock.Now().UTC()
	err = uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			q := uc.paymentAttempt.Tx(tx)
			locked, lockErr := q.LockByIDWithItems(ctx, attempt.ID)
			if lockErr != nil {
				return lockErr
			}

			if locked.FinancialCollectionObligationID != nil && strings.TrimSpace(*locked.FinancialCollectionObligationID) != "" {
				if strings.TrimSpace(*locked.FinancialCollectionObligationID) != obligationID {
					return checkoutPaymentSessionConflictError(
						"error.financial_core_conflicting_collection_obligation",
						nil,
						fmt.Errorf("%s: financial core returned conflicting collection obligation id", op),
					)
				}

				saved = locked
				return nil
			}

			saved, lockErr = q.BindCollectionObligationIfAbsent(ctx, locked.ID, obligationID, now)
			return lockErr
		})
	})
	if err != nil {
		return model.PaymentAttempt{}, err
	}

	return saved, nil
}

func (uc *UseCase) validateCheckoutPaymentResult(
	result extcheckoutpayment.CreateCheckoutPaymentResult,
	contractRow model.Contract,
	now time.Time,
) error {
	const op = "contractuc.UseCase.validateCheckoutPaymentResult"

	expectedAmountMinor, err := model.AmountMinorInt64(contractRow.PayableNowAmountMinor)
	if err != nil {
		return financialCoreCheckoutPaymentUnavailableError(fmt.Errorf("%s: checkout payable amount: %w", op, err))
	}

	if !contractRow.ExpiresAt.After(now) {
		return financialCoreCheckoutPaymentUnavailableError(
			errors.New("contract checkout payment expiry is invalid"),
		)
	}

	// Contract.ExpiresAt is the BNPL-requested CreateCheckoutPayment deadline.
	// An FC expiry later than that request is a soft warning, not a hard reject.
	err = extcheckoutpayment.ValidateRequiresActionCreateResult(
		result,
		extcheckoutpayment.RequiresActionCreateResultCheck{
			ExpectedAmountMinor: expectedAmountMinor,
			ExpectedCurrency:    string(contractRow.Currency),
			Now:                 now,
			MaxIdentifierLength: maxFinancialIdentifierLength,
			MaxPaymentURLLength: maxReturnURLLength,
		},
	)
	if err != nil {
		if errors.Is(err, extcheckoutpayment.ErrAmountCurrencyMismatch) {
			return financialCoreCheckoutPaymentConflictError(err)
		}

		return financialCoreCheckoutPaymentUnavailableError(err)
	}

	requestedExpiresAt := contractRow.ExpiresAt.UTC()
	extcheckoutpayment.WarnIfExpiresAtLaterThanRequested(
		uc.logger,
		requestedExpiresAt,
		result.ExpiresAt.UTC(),
		"contract_id", contractRow.ID.String(),
		"source_id", contractRow.ID.String(),
		"payment_operation_type", string(model.CheckoutFirstInstallmentFinancialCorePaymentPurpose),
	)

	return nil
}

func validateExternalPaymentURL(raw string) error {
	return extcheckoutpayment.ValidatePaymentRedirectURL(raw, maxReturnURLLength)
}

func validateConcurrentCheckoutPaymentSession(
	contractRow model.Contract,
	result extcheckoutpayment.CreateCheckoutPaymentResult,
	expiresAt time.Time,
	attempts ...model.PaymentAttempt,
) error {
	const op = "contractuc.validateConcurrentCheckoutPaymentSession"

	stored := checkoutPaymentOutputFromContract(contractRow)
	incoming := checkoutPaymentOutputFromResult(result, expiresAt)

	if stored.InvoiceID != incoming.InvoiceID ||
		stored.PaymentIntentID != incoming.PaymentIntentID ||
		stored.PaymentAttemptID != incoming.PaymentAttemptID ||
		stored.PaymentSessionID != incoming.PaymentSessionID ||
		stored.PaymentLink != incoming.PaymentLink ||
		stored.ExpiresAt != incoming.ExpiresAt {
		return checkoutPaymentSessionConflictError(
			"error.checkout_payment_session_financial_core_conflict",
			nil,
			fmt.Errorf("%s: financial core returned a conflicting checkout payment session for contract %s", op, contractRow.ID),
		)
	}

	if len(attempts) > 0 {
		attempt := attempts[0]
		paymentIntentID := strings.TrimSpace(result.PaymentIntentID)
		paymentSessionID := strings.TrimSpace(result.PaymentSessionID)
		if attempt.FinancialPaymentIntentID == nil || *attempt.FinancialPaymentIntentID != paymentIntentID ||
			attempt.FinancialPaymentSessionID == nil || *attempt.FinancialPaymentSessionID != paymentSessionID {
			return checkoutPaymentSessionConflictError(
				"error.checkout_payment_attempt_conflicts_with_stored_contract_session",
				nil,
				fmt.Errorf("%s: checkout payment attempt conflicts with stored contract session", op),
			)
		}
	}

	return nil
}

func (uc *UseCase) ensureCheckoutPaymentSession(
	ctx context.Context,
	contractRow model.Contract,
	attempt model.PaymentAttempt,
) (model.Contract, model.PaymentAttempt, CheckoutPaymentOutput, error) {
	attempt, err := uc.ensureActiveCheckoutPaymentAttempt(ctx, contractRow, attempt)
	if err != nil {
		return model.Contract{}, model.PaymentAttempt{}, CheckoutPaymentOutput{}, err
	}

	if checkoutContractSessionMatchesAttempt(contractRow, attempt) {
		if attempt.State == model.FailedPaymentAttemptState || attempt.State == model.CanceledPaymentAttemptState {
			return model.Contract{}, model.PaymentAttempt{}, CheckoutPaymentOutput{}, checkoutPaymentAttemptTerminalStateError(attempt.State)
		}

		if !contractRow.Status.AllowsCustomerPaymentContinuation() {
			return model.Contract{}, model.PaymentAttempt{}, CheckoutPaymentOutput{},
				contractStateCannotAcceptCheckoutPaymentSessionError(contractRow.Status)
		}

		// Identity agreement does not imply the gateway link is still usable.
		now := uc.clock.Now().UTC()
		if !model.CheckoutPaymentSessionUsableAt(contractRow, attempt, now) {
			return model.Contract{}, model.PaymentAttempt{}, CheckoutPaymentOutput{},
				checkoutPaymentSessionExpiredError(
					fmt.Errorf("stored checkout payment session expired at %s",
						contractRow.FinancialPaymentExpiresAt.UTC()),
				)
		}

		return contractRow, attempt, checkoutPaymentOutputFromContract(contractRow), nil
	}

	customer, merchant, err := uc.loadCheckoutFinancialParties(ctx, contractRow)
	if err != nil {
		return model.Contract{}, model.PaymentAttempt{}, CheckoutPaymentOutput{}, err
	}

	attempt, err = uc.ensureCheckoutCollectionObligation(
		ctx,
		contractRow,
		attempt,
		customer,
		merchant,
	)
	if err != nil {
		return model.Contract{}, model.PaymentAttempt{}, CheckoutPaymentOutput{}, err
	}

	request, err := uc.buildCheckoutPaymentInputFromAttempt(contractRow, attempt, customer.ExternalCustomerID)
	if err != nil {
		return model.Contract{}, model.PaymentAttempt{}, CheckoutPaymentOutput{}, err
	}

	paymentResult, err := uc.checkoutPaymentClient.CreateCheckoutPayment(ctx, request)
	if err != nil {
		return model.Contract{}, model.PaymentAttempt{}, CheckoutPaymentOutput{}, err
	}

	requestNow := uc.clock.Now().UTC()
	if err := uc.validateCheckoutPaymentResult(paymentResult, contractRow, requestNow); err != nil {
		return model.Contract{}, model.PaymentAttempt{}, CheckoutPaymentOutput{}, err
	}

	paymentIntentID := strings.TrimSpace(paymentResult.PaymentIntentID)
	paymentSessionID := strings.TrimSpace(paymentResult.PaymentSessionID)
	paymentAttemptID := strings.TrimSpace(paymentResult.PaymentAttemptID)
	invoiceID := strings.TrimSpace(paymentResult.InvoiceID)
	paymentLink := strings.TrimSpace(paymentResult.PaymentLink)
	expiresAt := paymentResult.ExpiresAt.UTC()
	reusedIdempotentResult := paymentResult.ReusedIdempotentResult

	var (
		savedContract             model.Contract
		savedAttempt              model.PaymentAttempt
		auditPreviousAttemptState *model.PaymentAttemptState
		postCommitErr             error
	)
	err = uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			contractQ := uc.contract.Tx(tx)
			attemptQ := uc.paymentAttempt.Tx(tx)
			outboxQ := uc.outbox.Tx(tx)

			lockedContract, lockErr := contractQ.LockByID(ctx, contractRow.ID)
			if lockErr != nil {
				return lockErr
			}

			lockedAttempt, lockErr := attemptQ.LockByIDWithItems(ctx, attempt.ID)
			if lockErr != nil {
				return lockErr
			}

			if lockedAttempt.ContractID != lockedContract.ID ||
				lockedAttempt.Purpose != model.CheckoutFirstInstallmentFinancialCorePaymentPurpose {
				return checkoutPaymentSessionConflictError(
					"error.checkout_payment_attempt_no_longer_matches_contract",
					nil,
					errors.New("checkout payment attempt no longer matches contract"),
				)
			}

			switch lockedAttempt.State {
			case model.CreatedPaymentAttemptState:
				submitted, submitErr := attemptQ.MarkSubmitted(
					ctx,
					lockedAttempt.ID,
					paymentIntentID,
					paymentSessionID,
					optionalTrimmedString(paymentLink),
					&expiresAt,
					requestNow,
					requestNow,
				)
				if submitErr != nil {
					return submitErr
				}

				savedAttempt = submitted
				previousState := lockedAttempt.State
				auditPreviousAttemptState = &previousState

			case model.SubmittedPaymentAttemptState:
				if !paymentAttemptSessionMatchesResult(lockedAttempt, paymentResult, expiresAt) {
					return checkoutPaymentSessionConflictError(
						"error.financial_core_conflicting_session_for_submitted_checkout_attempt",
						nil,
						errors.New("financial core returned conflicting session for submitted checkout attempt"),
					)
				}

				savedAttempt = lockedAttempt

			case model.ReconciliationRequiredPaymentAttemptState:
				if lockedAttempt.ReconciliationReason == nil ||
					*lockedAttempt.ReconciliationReason != model.PaymentSessionCreationTimeoutPaymentReconciliationReason {
					return checkoutPaymentAttemptReconciliationReasonCannotAcceptSessionError(
						reconciliationReasonValue(lockedAttempt.ReconciliationReason),
					)
				}

				submitted, submitErr := attemptQ.MarkSubmittedFromReconciliationRequired(
					ctx,
					lockedAttempt.ID,
					paymentIntentID,
					paymentSessionID,
					optionalTrimmedString(paymentLink),
					&expiresAt,
					requestNow,
					requestNow,
					requestNow,
				)
				if submitErr != nil {
					return submitErr
				}

				savedAttempt = submitted
				previousState := lockedAttempt.State
				auditPreviousAttemptState = &previousState

			case model.SucceededPaymentAttemptState,
				model.FailedPaymentAttemptState,
				model.CanceledPaymentAttemptState:
				enriched, enrichErr := attemptQ.EnrichFinancialPaymentSessionIfAbsent(
					ctx,
					lockedAttempt.ID,
					paymentIntentID,
					paymentSessionID,
					optionalTrimmedString(paymentAttemptID),
					paymentLink,
					expiresAt,
					requestNow,
				)
				if enrichErr != nil {
					return enrichErr
				}

				savedAttempt = enriched

				if lockedAttempt.State == model.FailedPaymentAttemptState ||
					lockedAttempt.State == model.CanceledPaymentAttemptState {
					postCommitErr = checkoutPaymentAttemptTerminalStateError(lockedAttempt.State)
				}

				if lockedContract.Status != model.PaymentPendingContractStatus &&
					lockedAttempt.State != model.SucceededPaymentAttemptState {
					savedContract = lockedContract
					return nil
				}

			default:
				return checkoutPaymentAttemptStateCannotAcceptSessionError(lockedAttempt.State)
			}

			if auditPreviousAttemptState != nil {
				if auditErr := uc.insertAttachCheckoutPaymentSessionAuditOutbox(
					ctx,
					outboxQ,
					attachCheckoutPaymentSessionAuditFacts{
						Contract:             lockedContract,
						PaymentAttempt:       savedAttempt,
						PreviousAttemptState: *auditPreviousAttemptState,
					},
					requestNow,
				); auditErr != nil {
					return auditErr
				}
			}

			currentAttempt, currentErr := attemptQ.GetByContractIDAndPurposeWithItems(
				ctx,
				lockedContract.ID,
				model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
			)
			if currentErr != nil {
				return currentErr
			}

			if currentAttempt.ID != lockedAttempt.ID {
				savedContract = lockedContract
				if postCommitErr == nil {
					postCommitErr = checkoutPaymentSessionConflictError(
						"error.checkout_payment_attempt_superseded_by_newer_retry",
						nil,
						errors.New("checkout payment attempt was superseded by a newer retry"),
					)
				}

				return nil
			}

			if contractHasFinancialCheckoutPaymentSession(lockedContract) {
				if concurrentErr := validateConcurrentCheckoutPaymentSession(
					lockedContract,
					paymentResult,
					expiresAt,
					savedAttempt,
				); concurrentErr == nil {
					savedContract = lockedContract
					return nil
				}

				if !isCheckoutPaymentRetryIdempotencyKey(lockedContract.ID, lockedAttempt.IdempotencyKey) {
					return checkoutPaymentSessionConflictError(
						"error.checkout_payment_session_financial_core_conflict",
						nil,
						errors.New("financial core returned a conflicting checkout payment session"),
					)
				}
			}

			canRepairAppliedSession := lockedAttempt.State == model.SucceededPaymentAttemptState &&
				isContractStateCompatibleWithAppliedCheckoutPayment(lockedContract.Status)
			if lockedContract.Status != model.PaymentPendingContractStatus && !canRepairAppliedSession {
				return contractStateCannotAcceptCheckoutPaymentSessionError(lockedContract.Status)
			}

			saved, saveErr := contractQ.SaveCheckoutPaymentSession(
				ctx,
				lockedContract.ID,
				invoiceID,
				paymentIntentID,
				paymentAttemptID,
				paymentSessionID,
				paymentLink,
				expiresAt,
				reusedIdempotentResult,
				requestNow,
			)
			if saveErr != nil {
				return saveErr
			}

			savedContract = saved
			return nil
		})
	})
	if err != nil {
		return model.Contract{}, model.PaymentAttempt{}, CheckoutPaymentOutput{}, err
	}

	if postCommitErr != nil {
		return model.Contract{}, model.PaymentAttempt{}, CheckoutPaymentOutput{}, postCommitErr
	}

	paymentOutput := checkoutPaymentOutputFromContract(savedContract)
	if paymentOutput.PaymentSessionID == "" {
		paymentOutput = checkoutPaymentOutputFromResult(paymentResult, expiresAt)
	}

	return savedContract, savedAttempt, paymentOutput, nil
}

func paymentAttemptSessionMatchesResult(
	attempt model.PaymentAttempt,
	result extcheckoutpayment.CreateCheckoutPaymentResult,
	expiresAt time.Time,
) bool {
	paymentIntentID := strings.TrimSpace(result.PaymentIntentID)
	paymentSessionID := strings.TrimSpace(result.PaymentSessionID)
	paymentLink := strings.TrimSpace(result.PaymentLink)
	return attempt.FinancialPaymentIntentID != nil && *attempt.FinancialPaymentIntentID == paymentIntentID &&
		attempt.FinancialPaymentSessionID != nil && *attempt.FinancialPaymentSessionID == paymentSessionID &&
		attempt.PaymentURL != nil && *attempt.PaymentURL == paymentLink &&
		attempt.FinancialPaymentExpiresAt != nil && attempt.FinancialPaymentExpiresAt.Equal(expiresAt)
}

func optionalStringValue(value *string) string {
	if value == nil {
		return ""
	}

	return strings.TrimSpace(*value)
}

func reconciliationReasonValue(reason *model.PaymentReconciliationReason) model.PaymentReconciliationReason {
	if reason == nil {
		return ""
	}

	return *reason
}

// CheckoutPaymentProjection projects persisted FC session metadata without
// submitting a new payment or re-entering Contract creation. A retrying hosted
// browser must never turn an ACTIVE Contract back into a payment request.
func CheckoutPaymentProjection(contract model.Contract) CheckoutPaymentOutput {
	return checkoutPaymentOutputFromContract(contract)
}
