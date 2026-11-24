package customercredituc

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy"
	"github.com/parstis/bnpl-backend/pkg/core/policy/initialcreditlimit"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// eligibleTrustedOwner validates an offline-imported mobile and national ID
// against authoritative CRS data resolved before the transaction.
//
// Callers invoke this before enroll-or-grant so a Trusted phone paired with the
// wrong CRS national code is a hard Conflict even when another account already
// holds person credit. Returning false means the mobile is not trusted and
// ordinary policy applies; mismatch never falls back to ordinary credit.
func (uc *UseCase) eligibleTrustedOwner(ctx context.Context, tx repo.Tx, profile model.CustomerProfile, mobile, nationalCode string) (bool, error) {
	const op = "customercredituc.UseCase.eligibleTrustedOwner"
	trustedMobileQ := uc.trustedMobile.Tx(tx)
	membership, err := trustedMobileQ.GetByMobile(ctx, mobile)
	if cerr.IsCode(err, cerr.CodeResourceNotFound) {
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("%s: lookup trusted mobile membership: %w", op, err)
	}

	if membership.NationalID != nationalCode {
		// The offline CSV is not a source of identity truth. CRS takes
		// precedence, and an immutable mismatch must fail closed. Report only
		// local IDs and a reason code: never log mobile or national ID.
		uc.logger.WithContext(ctx).Error(
			log.Text("credit.trusted_mobile_national_id_conflict", "trusted mobile import disagrees with authoritative CRS identity"),
			log.TextAttr("customer_id", profile.ID.String()),
			log.TextAttr("external_customer_id", profile.ExternalCustomerID.String()),
			log.TextAttr("reason_code", "TRUSTED_MOBILE_NATIONAL_ID_CONFLICT"),
		)
		return false, cerr.NewPublic(cerr.CodeConflict, "error.trusted_mobile_national_id_conflict",
			fmt.Errorf("trusted mobile and authoritative CRS national code do not match"),
			cerr.Details{Violations: []cerr.Violation{{Code: "TRUSTED_MOBILE_NATIONAL_ID_CONFLICT", MessageKey: "error.trusted_mobile_national_id_conflict"}}})
	}

	return true, nil
}

// trustedInstallmentPolicy selects the agreed 200m-toman v2 limit; checkout/v2
// is granted to the person in the SAME transaction that first creates credit.
func (uc *UseCase) trustedInstallmentPolicy() (initialcreditlimit.Policy, error) {
	return uc.initialLimitPolicies.Resolve(policy.Ref{ID: "customer_default_initial_limit", Version: "v2"})
}

func grantTrustedOwnerPolicies(ctx context.Context, q repo.PolicyAssignmentTxQueryer, owner model.CreditOwnerID, now time.Time) error {
	if owner.IsZero() {
		return fmt.Errorf("trusted owner identity missing")
	}

	for _, target := range policy.TrustedCustomerBundle() {
		// The owner row is already locked in all initial assignments.
		// No process may reassign someone else's owner policy implicitly.
		existing, err := q.GetActive(
			ctx,
			model.CreditOwnerPolicySubjectType,
			owner.String(),
			target.Family,
			now,
		)
		if err == nil {
			if existing.PolicyID != target.Ref.ID || existing.PolicyVersion != target.Ref.Version {
				return cerr.Conflict(fmt.Errorf("existing legal-person policy cannot be overwritten by trusted enrollment"))
			}

			continue
		}

		if !cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return err
		}

		_, err = q.Create(
			ctx,
			model.CreditOwnerPolicySubjectType,
			owner.String(),
			target.Family,
			target.Ref.ID,
			target.Ref.Version,
			"trusted_mobile_national_id_verified",
			"trusted_mobile_import",
			now,
			"bnpl",
			now,
			now,
		)
		if err != nil {
			return err
		}
	}

	return nil
}
