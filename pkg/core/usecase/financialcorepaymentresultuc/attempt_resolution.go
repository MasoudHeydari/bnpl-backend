package financialcorepaymentresultuc

import (
	"context"
	"strings"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/contractuc"
)

const financialCorePaymentAttemptSourceType = "bnpl_payment_attempt"

func resolvePaymentAttemptForResult(
	ctx context.Context,
	q repo.PaymentAttemptTxQueryer,
	input ProcessInput,
	purpose model.FinancialCorePaymentPurpose,
) (*model.PaymentAttempt, error) {
	if input.PaymentIntentID != "" {
		attempt, err := q.GetByFinancialPaymentIntentIDWithItems(ctx, input.PaymentIntentID)
		if err == nil {
			return &attempt, nil
		}

		if !cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return nil, err
		}
	}

	if strings.EqualFold(input.SourceType, financialCorePaymentAttemptSourceType) {
		attemptID, err := model.ParsePaymentAttemptID(input.SourceID)
		if err != nil || attemptID.IsZero() {
			return nil, sourceIDNotPaymentAttemptError(err)
		}

		attempt, err := q.GetByIDWithItems(ctx, attemptID)
		if err == nil {
			return &attempt, nil
		}

		if !cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return nil, err
		}
	}

	if purpose == model.CheckoutFirstInstallmentFinancialCorePaymentPurpose {
		contractID, err := resolveContractID(contractuc.ActivateFromFinancialCorePaymentResultInput{
			ContractID: input.ContractID,
			SourceType: input.SourceType,
			SourceID:   input.SourceID,
		})
		if err == nil {
			attempt, lookupErr := q.GetByContractIDAndPurposeWithItems(ctx, contractID, purpose)
			if lookupErr == nil {
				return &attempt, nil
			}

			if !cerr.IsCode(lookupErr, cerr.CodeResourceNotFound) {
				return nil, lookupErr
			}
		}
	}

	return nil, nil
}

func validateResultTargetsAttempt(
	attempt model.PaymentAttempt,
	input ProcessInput,
	purpose model.FinancialCorePaymentPurpose,
) error {
	if attempt.Purpose != purpose {
		return paymentPurposeMismatchConflictError()
	}

	if !input.ContractID.IsZero() && attempt.ContractID != input.ContractID {
		return contractIDMismatchConflictError()
	}

	if !input.InstallmentID.IsZero() && attempt.InstallmentID != input.InstallmentID {
		return installmentIDMismatchConflictError()
	}

	if strings.EqualFold(input.SourceType, financialCorePaymentAttemptSourceType) &&
		input.SourceID != attempt.ID.String() {
		return paymentResultSourceMismatchConflictError()
	}

	if attempt.FinancialPaymentIntentID != nil && strings.TrimSpace(*attempt.FinancialPaymentIntentID) != "" &&
		strings.TrimSpace(*attempt.FinancialPaymentIntentID) != input.PaymentIntentID {
		return paymentIntentMismatchConflictError()
	}

	if input.PaymentSessionID != "" && attempt.FinancialPaymentSessionID != nil &&
		strings.TrimSpace(*attempt.FinancialPaymentSessionID) != "" &&
		strings.TrimSpace(*attempt.FinancialPaymentSessionID) != input.PaymentSessionID {
		return paymentSessionMismatchConflictError()
	}

	return nil
}

func enrichAttemptIdentityFromResult(
	ctx context.Context,
	q repo.PaymentAttemptTxQueryer,
	attempt model.PaymentAttempt,
	input ProcessInput,
	now time.Time,
) (model.PaymentAttempt, error) {
	if input.PaymentIntentID == "" {
		return attempt, paymentIntentIDRequiredError()
	}

	sessionID := optionalNonEmptyString(input.PaymentSessionID)
	providerAttemptID := optionalProviderAttemptID(input.PaymentAttemptID)
	enriched, err := q.EnrichFinancialPaymentIdentityIfAbsent(
		ctx,
		attempt.ID,
		input.PaymentIntentID,
		sessionID,
		providerAttemptID,
		now,
	)
	if err != nil {
		return model.PaymentAttempt{}, err
	}

	enriched.Items = attempt.Items
	return enriched, nil
}

func optionalNonEmptyString(raw string) *string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil
	}

	return &value
}

func normalizedFailureCode(raw *string) *string {
	if raw == nil {
		return nil
	}

	value := strings.TrimSpace(*raw)
	if value == "" {
		return nil
	}

	return &value
}
