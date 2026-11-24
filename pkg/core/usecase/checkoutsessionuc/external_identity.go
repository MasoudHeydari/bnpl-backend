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
			fmt.Errorf("%s: current CRS mobile owner is different from the bound customer", op),
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

// isOpaqueMobileOwnershipFailure reports opaque fail-closed ownership errors
// that Partner Create maps to phone-scoped unavailable without leaking why.
func isOpaqueMobileOwnershipFailure(err error) bool {
	var appErr *cerr.Error
	if !errors.As(err, &appErr) {
		return false
	}

	return appErr.MessageKey == "error.checkout_otp_issuance_not_allowed"
}
