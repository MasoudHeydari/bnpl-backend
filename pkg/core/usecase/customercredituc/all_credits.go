package customercredituc

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// AllCreditStatus summarizes independent product assignments. Limits and
// available amounts must NEVER be added across products: monthly credit cannot
// finance a pay-in-four checkout or vice versa.
type AllCreditStatus string

const (
	AllCreditNotAssigned       AllCreditStatus = "not_assigned"
	AllCreditPartiallyAssigned AllCreditStatus = "partially_assigned"
	AllCreditAssigned          AllCreditStatus = "assigned"
)

var supportedCreditKinds = [...]model.CreditKind{
	model.PayInInstallmentsCreditKind,
	model.PayNextMonthCreditKind,
}

// AllSummariesOutput always includes every supported credit product,
// including products that have not yet been assigned.
type AllSummariesOutput struct {
	Status     AllCreditStatus
	CustomerID model.CustomerID
	Credits    []Output
}

// AggregateCreditStatus returns the overall state without treating the two
// limits as one fungible balance. Expired products remain visible and require
// reassessment, not silent omission.
func AggregateCreditStatus(credits []Output) AllCreditStatus {
	var active, assigned int
	for _, item := range credits {
		if item.Status == SummaryStatusActive {
			active++
		}

		if item.Status != SummaryStatusNotAssigned {
			assigned++
		}
	}

	if active == len(supportedCreditKinds) {
		return AllCreditAssigned
	}

	if assigned == 0 {
		return AllCreditNotAssigned
	}

	return AllCreditPartiallyAssigned
}

// GetAllSummaries resolves the authenticated customer once and reads both
// accounts in one local transaction. CreditUsage is determined by the same
// collections restriction evaluation for both products. Neither account
// inherits exposure, policy, or availability from the other.
func (uc *UseCase) GetAllSummaries(ctx context.Context, input Input) (AllSummariesOutput, error) {
	const op = "customercredituc.UseCase.GetAllSummaries"
	identity, err := uc.resolveActiveExternalCustomerIdentity(ctx, input.MobileE164)
	externalCustomerID := identity.ExternalCustomerID
	if err != nil {
		return AllSummariesOutput{}, err
	}

	now := uc.clock.Now().UTC()
	output := AllSummariesOutput{Credits: make([]Output, 0, len(supportedCreditKinds))}

	err = uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			customerProfileQ := uc.customerProfile.Tx(tx)
			policyAssignmentQ := uc.policyAssignment.Tx(tx)
			creditAccountQ := uc.creditAccount.Tx(tx)
			customerRestrictionQ := uc.customerRestriction.Tx(tx)
			installmentQ := uc.installment.Tx(tx)
			defaultEpisodeQ := uc.defaultEpisode.Tx(tx)

			profile, err := uc.ensureActiveCustomerProfile(
				ctx, customerProfileQ, input.SubjectID.AsExternalSubjectID(),
				externalCustomerID, identity.NationalCode, now,
			)
			if err != nil {
				return err
			}

			if !profile.IsActive() {
				return cerr.Conflict(fmt.Errorf("%s: customer profile is inactive", op))
			}

			output.CustomerID = profile.ID

			// Currency is determined by the customer's existing governed limit
			// policy, not by a caller-supplied amount or HTTP query.
			selectedPolicy, err := uc.resolveInitialLimitPolicy(
				ctx, policyAssignmentQ, externalCustomerID, now,
			)
			if err != nil {
				return err
			}

			currency := selectedPolicy.Terms().Currency
			products, ok := creditAccountQ.(repo.CreditAccountProductTxQueryer)
			if !ok {
				return cerr.InternalError(fmt.Errorf("%s: product-aware account repository unavailable", op))
			}

			// Read restrictions once for one consistent response. Restriction
			// truth is independent of assignment state.
			blocked, reason, err := newCreditUsageBlocked(
				ctx, customerRestrictionQ, installmentQ,
				defaultEpisodeQ, profile.ID, now, uc.collectionsPolicy,
			)
			if err != nil {
				return err
			}

			for _, kind := range supportedCreditKinds {
				monthlyBlocked := false
				if kind == model.PayNextMonthCreditKind {
					monthlyBlocked, err = monthlyPreviousCycleBlocked(
						ctx, products, profile.ID, currency, now,
					)
					if err != nil {
						return err
					}
				}

				if err := products.ExpireDueByCustomerIDCurrencyAndKind(
					ctx, profile.ID, currency, kind, now,
				); err != nil {
					return err
				}

				account, err := products.GetByCustomerIDCurrencyAndKind(
					ctx, profile.ID, currency, kind,
				)
				var item Output
				switch {
				case err == nil:
					item = summaryFromAccount(account, blocked || monthlyBlocked)
					item.CustomerID = profile.ID
					if blocked {
						item.CreditUsage.Reason = reason
					} else if monthlyBlocked {
						item.CreditUsage.Reason = "previous_month_unpaid"
					}

				case cerr.IsCode(err, cerr.CodeResourceNotFound):
					item = zeroSummary(profile.ID, currency)
				default:
					return err
				}
				item.CreditKind = kind
				output.Credits = append(output.Credits, item)
			}
			output.Status = AggregateCreditStatus(output.Credits)
			return nil
		})
	})
	if err != nil {
		return AllSummariesOutput{}, err
	}

	for _, item := range output.Credits {
		uc.observer.AvailabilityRetrieved(ctx, item.CustomerID, string(item.Status))
		if item.CreditUsage.Status == "restricted" {
			uc.observer.UsageRestricted(ctx, item.CustomerID, item.CreditUsage.Reason)
		}
	}

	return output, nil
}

// AllAssignmentsOutput describes the two independently persisted assessment
// results. Each credit product has its own immutable Decision and journal.
type AllAssignmentsOutput struct {
	Status  AllCreditStatus
	Credits []CreditAssignment
}

// CreditAssignment keeps the kind of each independent assignment explicit.
type CreditAssignment struct {
	CreditKind model.CreditKind
	Assignment AssignInitialLimitOutput
}

// InitiateAll assigns each product in turn, idempotently. This is a durable
// two-step application workflow, NOT an all-or-nothing financial transaction.
// If the second assessment fails, the first assignment remains valid; retrying
// the same request safely resumes the missing assessment.
func (uc *UseCase) InitiateAll(ctx context.Context, input AssignInitialLimitInput) (AllAssignmentsOutput, error) {
	output := AllAssignmentsOutput{Credits: make([]CreditAssignment, 0, len(supportedCreditKinds))}
	var active, assigned int
	for _, kind := range supportedCreditKinds {
		productInput := input
		productInput.CreditKind = kind
		assignment, err := uc.AssignInitialLimit(ctx, productInput)
		if err != nil {
			return AllAssignmentsOutput{}, err
		}

		output.Credits = append(output.Credits, CreditAssignment{
			CreditKind: kind,
			Assignment: assignment,
		})
		if assignment.Result == AssignInitialLimitResultCreated ||
			assignment.Result == AssignInitialLimitResultAlreadyAssigned {
			active++
			assigned++
		}
	}
	switch {
	case active == len(supportedCreditKinds):
		output.Status = AllCreditAssigned

	case assigned == 0:
		output.Status = AllCreditNotAssigned
	default:
		output.Status = AllCreditPartiallyAssigned
	}
	return output, nil
}
