package checkoutsessionuc

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// CreditKindPreviewReason is a merchant-safe, localizable unavailability reason.
type CreditKindPreviewReason struct {
	// Code is a stable machine-readable reason (never an internal stack detail).
	Code string
	// MessageKey is an i18n message key for the public response.
	MessageKey string
}

// CreditKindPreview describes one credit kind for Partner Create without
// creating a Decision or reserving exposure.
type CreditKindPreview struct {
	Kind      model.CreditKind
	Available bool
	Reason    *CreditKindPreviewReason
}

var previewCreditKinds = []model.CreditKind{
	model.PayIn4CreditKind,
	model.PayNextMonthCreditKind,
}

// buildCreditKindsPreview evaluates both supported credit kinds read-only.
// It never creates Decisions, Contracts, or claims exposure.
func (uc *UseCase) buildCreditKindsPreview(
	ctx context.Context,
	session model.CheckoutSession,
) ([]CreditKindPreview, error) {
	const op = "checkoutsessionuc.UseCase.buildCreditKindsPreview"

	previews := make([]CreditKindPreview, 0, len(previewCreditKinds))

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		merchantQ := uc.merchantProfile.Conn(conn)

		for _, kind := range previewCreditKinds {
			preview := CreditKindPreview{Kind: kind}

			enabled, err := merchantQ.IsCreditKindEnabled(ctx, session.MerchantProfileID, kind)
			if err != nil {
				return fmt.Errorf("%s: merchant eligibility for %s: %w", op, kind, err)
			}

			if !enabled {
				preview.Reason = &CreditKindPreviewReason{
					Code:       "MERCHANT_CREDIT_KIND_DISABLED",
					MessageKey: "error.credit_kind_not_enabled",
				}
				previews = append(previews, preview)

				continue
			}

			if session.CustomerID == nil || session.CustomerID.IsZero() {
				preview.Reason = &CreditKindPreviewReason{
					Code:       "CUSTOMER_NOT_RESOLVED",
					MessageKey: "error.customer_not_found",
				}
				previews = append(previews, preview)

				continue
			}

			products, ok := uc.creditAccount.Conn(conn).(repo.CreditAccountProductQueryer)
			if !ok {
				return cerr.InternalError(fmt.Errorf(
					"%s: product-aware credit account repository unavailable", op,
				))
			}

			account, err := products.GetByCustomerIDCurrencyAndKind(
				ctx, *session.CustomerID, session.Currency, kind,
			)
			if err != nil {
				if cerr.IsCode(err, cerr.CodeResourceNotFound) {
					preview.Reason = &CreditKindPreviewReason{
						Code:       "CREDIT_NOT_ASSIGNED",
						MessageKey: "error.credit_not_assigned",
					}
					previews = append(previews, preview)

					continue
				}

				return fmt.Errorf("%s: load credit account for %s: %w", op, kind, err)
			}

			now := uc.clock.Now().UTC()
			if !account.IsUsableAt(now) {
				preview.Reason = &CreditKindPreviewReason{
					Code:       "CREDIT_ACCOUNT_UNUSABLE",
					MessageKey: "error.credit_account_unusable",
				}
				previews = append(previews, preview)

				continue
			}

			if account.AvailableAmountMinor() < session.RequestedAmountMinor {
				preview.Reason = &CreditKindPreviewReason{
					Code:       "INSUFFICIENT_AVAILABLE_CREDIT",
					MessageKey: "error.insufficient_available_credit",
				}
				previews = append(previews, preview)

				continue
			}

			preview.Available = true
			previews = append(previews, preview)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return previews, nil
}
