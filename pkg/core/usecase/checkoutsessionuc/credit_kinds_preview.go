package checkoutsessionuc

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/calendar/jalali"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout/businessrule"
	checkoutcatalog "github.com/parstis/bnpl-backend/pkg/core/policy/checkout/catalog"
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
	Kind             model.CreditKind
	Available        bool
	Reason           *CreditKindPreviewReason
	RepaymentPlan    model.RepaymentPlanKind
	InstallmentCount int
}

var previewCreditKinds = []model.CreditKind{
	model.PayInInstallmentsCreditKind,
	model.PayNextMonthCreditKind,
}

// buildCreditKindsPreview evaluates both supported credit kinds read-only.
// It never creates Decisions, Contracts, or claims exposure. Installment
// repayment-plan metadata reflects the customer's governing Checkout Policy
// (assignment or catalog default). Launch business-rule mismatches (for
// example ordinary four-installment upfront checkout) are reported unavailable.
func (uc *UseCase) buildCreditKindsPreview(
	ctx context.Context,
	session model.CheckoutSession,
) ([]CreditKindPreview, error) {
	const op = "checkoutsessionuc.UseCase.buildCreditKindsPreview"

	previews := make([]CreditKindPreview, 0, len(previewCreditKinds))

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		merchantQ := uc.merchantProfile.Conn(conn)
		customerQ := uc.customerProfile.Conn(conn)
		assignmentQ := uc.policyAssignment.Conn(conn)
		installmentQ := uc.installment.Conn(conn)

		var (
			externalCustomerID model.ExternalCustomerID
			hasCustomer        bool
			customerBlocked    *CreditKindPreviewReason
		)

		if session.CustomerID != nil && !session.CustomerID.IsZero() {
			profile, profileErr := customerQ.GetByID(ctx, *session.CustomerID)
			if profileErr != nil {
				return fmt.Errorf("%s: load customer profile: %w", op, profileErr)
			}

			externalCustomerID = profile.ExternalCustomerID
			hasCustomer = true

			if !profile.IsActive() {
				customerBlocked = &CreditKindPreviewReason{
					Code:       "CUSTOMER_NOT_ACTIVE",
					MessageKey: "error.customer_profile_not_active",
				}
			}
		}

		now := uc.clock.Now().UTC()

		// Customer-wide due/overdue obligations block every credit kind, matching
		// Decision's AssertNewCreditAllowed due predicate (now >= DueAt).
		if hasCustomer && customerBlocked == nil {
			dueUnpaid, dueErr := installmentQ.ListDueUnpaidByCustomerID(
				ctx, *session.CustomerID, now,
			)
			if dueErr != nil {
				return fmt.Errorf("%s: list due unpaid installments: %w", op, dueErr)
			}

			if len(dueUnpaid) > 0 {
				customerBlocked = &CreditKindPreviewReason{
					Code:       "NEW_CREDIT_USAGE_RESTRICTED",
					MessageKey: "error.collections_new_credit_usage_restricted",
				}
			}
		}

		var branchBlocked *CreditKindPreviewReason
		if !session.MerchantBranchProfileID.IsZero() {
			branchQ := uc.merchantBranchProfile.Conn(conn)
			branch, branchErr := branchQ.GetByID(ctx, session.MerchantBranchProfileID)
			if branchErr != nil {
				return fmt.Errorf("%s: load merchant branch: %w", op, branchErr)
			}

			if branch.Status != model.ActiveMerchantBranchStatus {
				branchBlocked = &CreditKindPreviewReason{
					Code:       "MERCHANT_BRANCH_NOT_ACTIVE",
					MessageKey: "error.merchant_branch_not_active",
				}
			}
		}

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

			if !hasCustomer {
				preview.Reason = &CreditKindPreviewReason{
					Code:       "CUSTOMER_NOT_RESOLVED",
					MessageKey: "error.customer_not_found",
				}
				previews = append(previews, preview)

				continue
			}

			if branchBlocked != nil {
				reason := *branchBlocked
				preview.Reason = &reason
				previews = append(previews, preview)

				continue
			}

			if customerBlocked != nil {
				reason := *customerBlocked
				preview.Reason = &reason
				previews = append(previews, preview)

				continue
			}

			selected, policyErr := uc.resolvePreviewCheckoutPolicy(
				ctx,
				assignmentQ,
				externalCustomerID,
				kind,
				now,
			)
			if policyErr != nil {
				return fmt.Errorf("%s: resolve policy for %s: %w", op, kind, policyErr)
			}

			terms := selected.Terms()
			preview.InstallmentCount = terms.InstallmentCount

			plan, ok, planErr := checkout.RepaymentPlan(terms)
			if planErr != nil {
				return fmt.Errorf("%s: repayment plan for %s: %w", op, kind, planErr)
			}

			if ok {
				preview.RepaymentPlan = plan
			}

			if err := businessrule.Active().ValidatePolicyTerms(terms); err != nil {
				preview.Reason = &CreditKindPreviewReason{
					Code:       "CHECKOUT_UPFRONT_PAYMENT_UNAVAILABLE",
					MessageKey: "error.checkout_upfront_payment_unavailable",
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

			if model.EffectiveCreditKind(kind) == model.PayNextMonthCreditKind {
				cutoff, cutoffErr := jalali.StartOfFollowingTehranMonth(now)
				if cutoffErr != nil {
					return fmt.Errorf("%s: monthly cycle cutoff: %w", op, cutoffErr)
				}

				blocked, cycleErr := products.HasUnpaidMonthlyCycleBefore(
					ctx, *session.CustomerID, session.Currency, cutoff,
				)
				if cycleErr != nil {
					return fmt.Errorf("%s: monthly cycle check: %w", op, cycleErr)
				}

				if blocked {
					preview.Reason = &CreditKindPreviewReason{
						Code:       "MONTHLY_PREVIOUS_CYCLE_UNPAID",
						MessageKey: "error.monthly_previous_cycle_unpaid",
					}
					previews = append(previews, preview)

					continue
				}
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

func (uc *UseCase) resolvePreviewCheckoutPolicy(
	ctx context.Context,
	assignmentQ repo.PolicyAssignmentQueryer,
	externalCustomerID model.ExternalCustomerID,
	kind model.CreditKind,
	now time.Time,
) (checkout.Policy, error) {
	const op = "checkoutsessionuc.UseCase.resolvePreviewCheckoutPolicy"

	if model.EffectiveCreditKind(kind) == model.PayNextMonthCreditKind {
		selected, err := uc.checkoutPolicies.ForCreditKind(model.PayNextMonthCreditKind)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}

		return selected, nil
	}

	ref := checkoutcatalog.DefaultRef()
	assignment, err := assignmentQ.GetActive(
		ctx,
		model.CustomerPolicySubjectType,
		externalCustomerID.String(),
		model.CheckoutPolicyFamily,
		now,
	)
	if err == nil {
		ref = policy.Ref{ID: assignment.PolicyID, Version: assignment.PolicyVersion}
	} else if !cerr.IsCode(err, cerr.CodeResourceNotFound) {
		return nil, fmt.Errorf("%s: load active assignment: %w", op, err)
	}

	selected, err := uc.checkoutPolicies.Resolve(ref)
	if err != nil {
		return nil, fmt.Errorf("%s: resolve policy %s: %w", op, ref, err)
	}

	return selected, nil
}
