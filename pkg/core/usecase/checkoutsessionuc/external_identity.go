package checkoutsessionuc

import (
	"context"
	"errors"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/external/customer"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// errCheckoutMobileOwnerChanged marks a definitive CRS reassignment or phone
// removal. Temporary CRS failures are deliberately not classified as a
// reassignment: they must never release a purchase claim.
var errCheckoutMobileOwnerChanged = errors.New("checkout mobile no longer belongs to bound customer")

// boundExternalMobileOwnership is the CRS-authoritative owner of the
// merchant-asserted phone for a session that already has a local CustomerID
// binding. OTP proves possession of a still-valid challenge for that session;
// it does not itself re-prove CRS ownership. Callers must revalidate with CRS
// outside any DB transaction before Prepare replay, Start, OTP issue, and
// Confirm, then compare the locked binding to these expected identifiers.
type boundExternalMobileOwnership struct {
	CustomerID         model.CustomerID
	ExternalCustomerID model.ExternalCustomerID
	Phone              string
}

// revalidateBoundExternalMobileOwnership asks CRS who currently owns the
// session phone and fails closed when that owner is not the bound BNPL
// profile. Network CRS work never runs inside a transaction.
//
// Public errors stay opaque so remapping cannot be distinguished from other
// issuance/continuation failures (no account enumeration).
func (uc *UseCase) revalidateBoundExternalMobileOwnership(
	ctx context.Context,
	session model.CheckoutSession,
) (boundExternalMobileOwnership, error) {
	const op = "checkoutsessionuc.UseCase.revalidateBoundExternalMobileOwnership"

	if session.CustomerID == nil || session.CustomerID.IsZero() {
		return boundExternalMobileOwnership{}, checkoutOTPIssuanceNotAllowedError(
			fmt.Errorf("%s: customer binding is missing", op),
		)
	}

	phone, err := uc.decryptCustomerPhone(session)
	if err != nil {
		return boundExternalMobileOwnership{}, err
	}

	resolved, err := uc.customerResolver.ResolveByMobile(
		ctx,
		customer.ResolveByMobileInput{MobileE164: phone},
	)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return boundExternalMobileOwnership{}, checkoutOTPIssuanceNotAllowedError(
				fmt.Errorf("%s: CRS mobile no longer resolves: %w", op, errCheckoutMobileOwnerChanged),
			)
		}

		return boundExternalMobileOwnership{}, checkoutOTPIssuanceNotAllowedError(
			fmt.Errorf("%s: CRS revalidation failed: %w", op, err),
		)
	}

	if resolved.Status != customer.ActiveStatus || resolved.ExternalCustomerID.IsZero() {
		return boundExternalMobileOwnership{}, checkoutOTPIssuanceNotAllowedError(
			fmt.Errorf("%s: phone is not linked to an active customer", op),
		)
	}

	var profile model.CustomerProfile
	err = uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		customerProfileQ := uc.customerProfile.Conn(conn)
		var getErr error
		profile, getErr = customerProfileQ.GetByID(ctx, *session.CustomerID)
		return getErr
	})
	if err != nil {
		return boundExternalMobileOwnership{}, checkoutOTPIssuanceNotAllowedError(
			fmt.Errorf("%s: load bound customer: %w", op, err),
		)
	}

	if profile.ExternalCustomerID != resolved.ExternalCustomerID {
		return boundExternalMobileOwnership{}, checkoutOTPIssuanceNotAllowedError(
			fmt.Errorf("%s: CRS mobile owner differs from bound account: %w", op, errCheckoutMobileOwnerChanged),
		)
	}

	return boundExternalMobileOwnership{
		CustomerID:         *session.CustomerID,
		ExternalCustomerID: profile.ExternalCustomerID,
		Phone:              phone,
	}, nil
}

// assertLockedBindingMatchesOwnership compares identifiers under a held
// transaction lock to a CRS preflight taken outside the transaction.
func assertLockedBindingMatchesOwnership(
	op string,
	lockedCustomerID model.CustomerID,
	lockedExternalCustomerID model.ExternalCustomerID,
	expected boundExternalMobileOwnership,
) error {
	if lockedCustomerID != expected.CustomerID ||
		lockedExternalCustomerID != expected.ExternalCustomerID {
		return checkoutCustomerIdentityConflictError(
			fmt.Errorf(
				"%s: locked binding does not match CRS preflight",
				op,
			),
		)
	}

	return nil
}

// ensureBoundSessionMobileOwnership revalidates CRS ownership for a bound
// session. Unbound sessions are a no-op (caller still resolves via CRS).
func (uc *UseCase) ensureBoundSessionMobileOwnership(
	ctx context.Context,
	session model.CheckoutSession,
) error {
	if session.CustomerID == nil || session.CustomerID.IsZero() {
		return nil
	}

	_, err := uc.revalidateBoundExternalMobileOwnership(ctx, session)
	return err
}

// revalidateBoundExternalMobileOwnershipForCommand rejects an invalid phone
// and retires a definitively reassigned, still-open checkout when safe. Reads
// use the non-mutating helper instead. Temporary CRS errors retain the claim.
func (uc *UseCase) revalidateBoundExternalMobileOwnershipForCommand(
	ctx context.Context,
	session model.CheckoutSession,
) (boundExternalMobileOwnership, error) {
	ownership, err := uc.revalidateBoundExternalMobileOwnership(ctx, session)
	if err != nil {
		if terminateErr := uc.terminateReassignedCheckoutIfSafe(ctx, session, err); terminateErr != nil {
			return boundExternalMobileOwnership{}, terminateErr
		}
		return boundExternalMobileOwnership{}, err
	}
	return ownership, nil
}

// isOpaqueMobileOwnershipFailure reports opaque fail-closed ownership errors
// that Partner Create maps to phone-scoped unavailable without leaking why.
func isOpaqueMobileOwnershipFailure(err error) bool {
	var appErr *cerr.Error
	if !errors.As(err, &appErr) {
		return false
	}

	return appErr.MessageKey == "error.checkout_otp_issuance_not_allowed"
}

// canRetireReassignedCheckout deliberately excludes confirmation and payment
// recovery. CRS ownership is not evidence about a previously authorized FC
// operation. Those sessions continue through the existing saga/reconciliation.
func canRetireReassignedCheckout(session model.CheckoutSession) bool {
	return session.Status == model.OpenCheckoutSessionStatus &&
		session.ContractID == nil && session.ConfirmationAttemptID == nil &&
		session.CustomerID != nil && !session.CustomerID.IsZero()
}

// terminateReassignedCheckoutIfSafe retires only an identity-invalidated
// checkout whose financial effects can be PROVEN absent. Reusing the shared
// safe-termination gate fences a possible confirmation saga/contract. A CRS
// timeout or suspended account never reaches this operation.
func (uc *UseCase) terminateReassignedCheckoutIfSafe(
	ctx context.Context,
	session model.CheckoutSession,
	identityErr error,
) error {
	const op = "checkoutsessionuc.UseCase.terminateReassignedCheckoutIfSafe"

	if !errors.Is(identityErr, errCheckoutMobileOwnerChanged) ||
		!canRetireReassignedCheckout(session) {
		return nil
	}

	_, err := uc.TerminateCheckoutSafely(ctx, TerminateCheckoutSafelyInput{
		SessionID:         session.ID,
		Reason:            model.TechnicalFailureCheckoutReason,
		TerminalStatus:    model.CanceledCheckoutSessionStatus,
		reassignedBinding: &session,
	})
	if err != nil {
		// Inconclusive FC effects or a concurrent Confirm must retain the
		// purchase claim. Never bypass the existing safe-termination checks.
		return fmt.Errorf("%s: cannot safely close reassigned mobile checkout: %w", op, err)
	}

	return nil
}
