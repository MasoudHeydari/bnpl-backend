package customercredituc

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// GetSummary returns the authenticated customer's current BNPL credit summary.
//
// CreditAccount is the single current-state source for both governed credit
// capacity and committed financed exposure. The immutable credit_transactions
// journal explains how that state changed; it is not recomputed as a financial
// ledger.
func (uc *UseCase) GetSummary(ctx context.Context, input Input) (Output, error) {
	const op = "customercredituc.UseCase.GetSummary"

	kind := model.EffectiveCreditKind(input.CreditKind)
	if !kind.IsValid() {
		return Output{}, cerr.InvalidArgument(fmt.Errorf("unsupported credit_kind %q", input.CreditKind))
	}

	identity, err := uc.resolveActiveExternalCustomerIdentity(ctx, input.MobileE164)
	externalCustomerID := identity.ExternalCustomerID
	if err != nil {
		return Output{}, err
	}

	now := uc.clock.Now().UTC()
	var output Output

	err = uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			customerProfileQ := uc.customerProfile.Tx(tx)
			profile, err := uc.ensureActiveCustomerProfile(
				ctx,
				customerProfileQ,
				input.SubjectID.AsExternalSubjectID(),
				externalCustomerID,
				identity.NationalCode,
				now,
			)
			if err != nil {
				return err
			}

			accountQ := uc.creditAccount.Tx(tx)
			policyAssignmentQ := uc.policyAssignment.Tx(tx)
			selectedPolicy, err := uc.resolveInitialLimitPolicy(ctx, policyAssignmentQ, externalCustomerID, now)
			if err != nil {
				return err
			}

			currency := selectedPolicy.Terms().Currency
			request, err := uc.latestCreditRequest(ctx, tx, profile.ID, kind)
			if err != nil {
				return err
			}
			var account model.CreditAccount
			if kind == model.PayInInstallmentsCreditKind {
				if err = accountQ.ExpireDueByCustomerIDAndCurrency(ctx, profile.ID, currency, now); err != nil {
					return err
				}

				account, err = accountQ.GetByCustomerIDAndCurrency(ctx, profile.ID, currency)
			} else {
				if err = accountQ.ExpireDueByCustomerIDCurrencyAndKind(ctx, profile.ID, currency, kind, now); err != nil {
					return err
				}

				account, err = accountQ.GetByCustomerIDCurrencyAndKind(ctx, profile.ID, currency, kind)
			}

			if err != nil {
				if cerr.IsCode(err, cerr.CodeResourceNotFound) {
					output = zeroSummary(profile.ID, currency)
					output.CreditKind = kind
					output.CreditRequest = request
					return nil
				}

				return err
			}

			restrictionQ := uc.customerRestriction.Tx(tx)
			installmentQ := uc.installment.Tx(tx)
			episodeQ := uc.defaultEpisode.Tx(tx)
			newCreditBlocked, publicReason, blockErr := newCreditUsageBlocked(
				ctx,
				restrictionQ,
				installmentQ,
				episodeQ,
				profile.ID,
				now, uc.collectionsPolicy,
			)
			if blockErr != nil {
				return blockErr
			}

			// Old monthly debt blocks new monthly purchases from day 1 of
			// the next Jalali month, even before the due date on day 5.
			monthlyBlocked := false
			if kind == model.PayNextMonthCreditKind {
				monthlyBlocked, err = monthlyPreviousCycleBlocked(
					ctx, accountQ, profile.ID, currency, now,
				)
				if err != nil {
					return err
				}
			}

			output = summaryFromAccount(account, newCreditBlocked || monthlyBlocked)
			output.CustomerID = profile.ID
			output.CreditRequest = request
			output.CreditKind = kind
			if newCreditBlocked {
				output.CreditUsage.Reason = publicReason
			} else if monthlyBlocked {
				output.CreditUsage.Reason = "previous_month_unpaid"
			}
			return nil
		})
	})
	if err != nil {
		return Output{}, err
	}

	uc.observer.AvailabilityRetrieved(ctx, output.CustomerID, string(output.Status))

	if output.CreditUsage.Status == "restricted" {
		uc.observer.UsageRestricted(ctx, output.CustomerID, output.CreditUsage.Reason)
	}

	return output, nil
}

func summaryFromAccount(account model.CreditAccount, newCreditBlocked bool) Output {
	status := SummaryStatusActive
	available := account.AvailableAmountMinor()
	if !account.Status.IsActive() {
		status = SummaryStatusExpired
		available = 0
	}

	// Stored capacity/exposure remain authoritative on CreditAccount; usable
	// capacity is zero while an active new-credit restriction (or live default
	// eligibility) blocks spendable credit.
	if newCreditBlocked {
		available = 0
	}

	usage := CreditUsage{Status: "allowed"}
	if newCreditBlocked {
		// Reason is overwritten by the caller with the public restriction reason
		// (collections_due, collections_default, credit_usage_restricted, …).
		usage = CreditUsage{Status: "restricted"}
	}

	if status != SummaryStatusActive && !newCreditBlocked {
		usage = CreditUsage{Status: "unavailable"}
	}

	return Output{
		CreditUsage:          usage,
		CustomerID:           account.CustomerID,
		LimitAmountMinor:     account.LimitAmountMinor,
		AvailableAmountMinor: available,
		Currency:             account.Currency,
		Status:               status,
		LimitStatus:          &account.Status,
		ExpiresAt:            account.ExpiresAt,
	}
}

func zeroSummary(customerID model.CustomerID, currency model.Currency) Output {
	return Output{
		CreditUsage:          CreditUsage{Status: "unavailable"},
		CustomerID:           customerID,
		LimitAmountMinor:     0,
		AvailableAmountMinor: 0,
		Currency:             currency,
		Status:               SummaryStatusNotAssigned,
		LimitStatus:          nil,
		ExpiresAt:            nil,
	}
}

// ensureActiveCustomerProfile creates an active IR customer profile when missing.
// Existing profiles are never overwritten or reactivated.
func (uc *UseCase) ensureActiveCustomerProfile(
	ctx context.Context,
	profileQ repo.CustomerProfileTxQueryer,
	externalSubjectID model.ExternalSubjectID,
	externalCustomerID model.ExternalCustomerID,
	nationalCode string,
	now time.Time,
) (model.CustomerProfile, error) {
	profile, _, err := profileQ.EnsureExists(
		ctx,
		externalSubjectID,
		externalCustomerID,
		model.IRJurisdiction,
		model.ActiveCustomerStatus,
		now,
		now,
		nationalCode,
	)
	if err != nil {
		return model.CustomerProfile{}, err
	}

	return profile, nil
}
