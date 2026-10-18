package customercredituc

import (
	"context"
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
	if input.ExternalCustomerID.IsZero() {
		return Output{}, externalCustomerIDRequiredError()
	}

	now := uc.clock.Now().UTC()
	var output Output

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			customerProfileQ := uc.customerProfile.Tx(tx)
			profile, err := uc.ensureActiveCustomerProfile(
				ctx,
				customerProfileQ,
				input.ExternalCustomerID,
				now,
			)
			if err != nil {
				return err
			}

			accountQ := uc.creditAccount.Tx(tx)
			policyAssignmentQ := uc.policyAssignment.Tx(tx)
			selectedPolicy, err := uc.resolveInitialLimitPolicy(ctx, policyAssignmentQ, input.ExternalCustomerID, now)
			if err != nil {
				return err
			}
			currency := selectedPolicy.Terms().Currency
			if err = accountQ.ExpireDueByCustomerIDAndCurrency(ctx, profile.ID, currency, now); err != nil {
				return err
			}

			account, err := accountQ.GetByCustomerIDAndCurrency(ctx, profile.ID, currency)
			if err != nil {
				if cerr.IsCode(err, cerr.CodeResourceNotFound) {
					output = zeroSummary(profile.ID, currency)
					return nil
				}

				return err
			}

			output = summaryFromAccount(account)
			return nil
		})
	})
	if err != nil {
		return Output{}, err
	}

	return output, nil
}

func summaryFromAccount(account model.CreditAccount) Output {
	status := SummaryStatusActive
	available := account.AvailableAmountMinor()
	if !account.Status.IsActive() {
		status = SummaryStatusExpired
		available = 0
	}

	return Output{
		CustomerID:           account.CustomerID,
		AvailableAmountMinor: available,
		Currency:             account.Currency,
		Status:               status,
		LimitStatus:          &account.Status,
		ExpiresAt:            account.ExpiresAt,
	}
}

func zeroSummary(customerID model.CustomerID, currency model.Currency) Output {
	return Output{
		CustomerID:           customerID,
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
	externalCustomerID model.ExternalCustomerID,
	now time.Time,
) (model.CustomerProfile, error) {
	profile, _, err := profileQ.EnsureExists(
		ctx,
		externalCustomerID,
		model.IRJurisdiction,
		model.ActiveCustomerStatus,
		now,
		now,
	)
	if err != nil {
		return model.CustomerProfile{}, err
	}

	return profile, nil
}
