package customercredituc

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// lockCreditOwnerForAssignment serializes grant decisions of two accounts with
// one legal owner. A missing or mismatched owner is an identity conflict, never
// an opportunity to skip the lock.
func (uc *UseCase) lockCreditOwnerForAssignment(
	ctx context.Context,
	tx repo.Tx,
	profile model.CustomerProfile,
) error {
	const op = "customercredituc.UseCase.lockCreditOwnerForAssignment"
	if profile.CreditOwnerID.IsZero() {
		return cerr.InternalError(fmt.Errorf("%s: legal-person identity is missing", op))
	}

	ownerID, err := uc.sharedCredit.LockOwnerByCustomer(ctx, tx, profile.ID)
	if err != nil {
		return fmt.Errorf("%s: lock owner: %w", op, err)
	}

	if ownerID != profile.CreditOwnerID {
		return cerr.Conflict(fmt.Errorf("%s: customer identity disagrees with the locked legal person", op))
	}

	return nil
}

// enrollExistingSharedCredit is exclusively used by authenticated credit
// initiation. It grants only per-account access and cannot increase the shared
// limit; GET and merchant Checkout never call it.
func (uc *UseCase) enrollExistingSharedCredit(
	ctx context.Context,
	tx repo.Tx,
	profile model.CustomerProfile,
	currency model.Currency,
	kind model.CreditKind,
	now time.Time,
) (*AssignInitialLimitOutput, error) {
	const op = "customercredituc.UseCase.enrollExistingSharedCredit"
	if profile.CreditOwnerID.IsZero() {
		return nil, cerr.InternalError(fmt.Errorf("%s: legal-person identity is missing", op))
	}

	account, err := uc.sharedCredit.LockByOwnerAndKind(
		ctx,
		tx,
		profile.CreditOwnerID,
		currency,
		kind,
	)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return nil, nil
		}

		return nil, fmt.Errorf("%s: load person credit: %w", op, err)
	}

	if !account.IsUsableAt(now) {
		// Another account cannot renew an expired or revoked person's grant.
		if account.CustomerID != profile.ID {
			return nil, cerr.Conflict(fmt.Errorf("%s: legal person already has non-usable credit", op))
		}

		return nil, nil
	}

	_, err = uc.sharedCredit.EnrollCustomer(
		ctx,
		tx,
		profile.ID,
		account.ID,
		profile.CreditOwnerID,
		now,
	)
	if err != nil {
		return nil, fmt.Errorf("%s: enroll account: %w", op, err)
	}

	decisionQ := uc.decision.Tx(tx)
	output, err := uc.outputFromExistingCreditAccount(ctx, decisionQ, account)
	if err != nil {
		return nil, err
	}

	output.Result = AssignInitialLimitResultAlreadyAssigned
	// Another account may reuse person-level capacity, not the originating
	// account's credit assessment explanations or private decision reasons.
	if account.CustomerID != profile.ID {
		output.Decision.ReasonCodes = nil
	}

	if output.CreditLimit != nil {
		output.CreditLimit.CustomerID = profile.ID
	}

	return &output, nil
}
