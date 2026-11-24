package customercredituc

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	extcustomer "github.com/parstis/bnpl-backend/pkg/core/external/customer"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// resolveActiveExternalCustomerIdentity canonicalizes the gateway mobile, resolves it
// through Customer Reference Service outside any DB transaction, and returns
// the CRS customer_id only when status is ACTIVE.
//
// That CRS id is persisted as customer_profiles.external_customer_id and is the
// subject key for initial-credit policy assignments (including trusted v2).
type resolvedCustomerIdentity struct {
	ExternalCustomerID model.ExternalCustomerID
	NationalCode       string
}

var crsNationalCodePattern = regexp.MustCompile(`^[0-9]{10}$`)

func (uc *UseCase) resolveActiveExternalCustomerIdentity(
	ctx context.Context,
	mobileE164 string,
) (resolvedCustomerIdentity, error) {
	const op = "customercredituc.UseCase.resolveActiveExternalCustomerIdentity"

	mobile, err := normalizeAuthMobileE164(mobileE164)
	if err != nil {
		return resolvedCustomerIdentity{}, err
	}

	resolved, err := uc.customerResolver.ResolveByMobile(
		ctx,
		extcustomer.ResolveByMobileInput{MobileE164: mobile},
	)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return resolvedCustomerIdentity{}, customerReferenceNotFoundError(err)
		}

		return resolvedCustomerIdentity{}, fmt.Errorf("%s: resolve customer by mobile: %w", op, err)
	}

	if resolved.ExternalCustomerID.IsZero() {
		return resolvedCustomerIdentity{}, cerr.InternalError(
			fmt.Errorf("%s: customer reference service returned an empty customer id", op),
		)
	}

	switch resolved.Status {
	case extcustomer.ActiveStatus:
		uc.observer.CustomerResolved(ctx, resolved.ExternalCustomerID)

		if !crsNationalCodePattern.MatchString(resolved.NationalCode) {
			return resolvedCustomerIdentity{}, cerr.InternalError(fmt.Errorf("%s: Customer Reference Service returned an invalid national code", op))
		}
		return resolvedCustomerIdentity{ExternalCustomerID: resolved.ExternalCustomerID, NationalCode: resolved.NationalCode}, nil

	case extcustomer.SuspendedStatus:
		return resolvedCustomerIdentity{}, customerReferenceNotActiveError(
			fmt.Errorf("%s: customer reference status is suspended", op),
		)

	default:
		return resolvedCustomerIdentity{}, cerr.InternalError(
			fmt.Errorf("%s: unsupported customer reference status %q", op, resolved.Status),
		)
	}
}

// resolveActiveExternalCustomerID is retained for callers that only need the
// account identifier; all customer creation paths must use the full identity.
func (uc *UseCase) resolveActiveExternalCustomerID(ctx context.Context, mobile string) (model.ExternalCustomerID, error) {
	result, err := uc.resolveActiveExternalCustomerIdentity(ctx, mobile)
	if err != nil {
		return model.ExternalCustomerID{}, err
	}
	return result.ExternalCustomerID, nil
}

// normalizeAuthMobileE164 mirrors the gateway E.164 contract for
// auth_mobile_number / x-auth-mobile-number: non-empty, leading '+', digits
// only after '+', length 9..16 inclusive.
func normalizeAuthMobileE164(raw string) (string, error) {
	mobile := strings.TrimSpace(raw)
	if mobile == "" {
		return "", authMobileNumberRequiredError()
	}

	if !strings.HasPrefix(mobile, "+") {
		return "", authMobileNumberInvalidError()
	}

	digits := mobile[1:]
	if len(mobile) < 9 || len(mobile) > 16 {
		return "", authMobileNumberInvalidError()
	}

	for _, digit := range digits {
		// The public phone boundary accepts only ASCII E.164 digits. Unicode
		// numerals are not interchangeable with the canonical DB/mobile key.
		if digit < '0' || digit > '9' {
			return "", authMobileNumberInvalidError()
		}
	}

	return mobile, nil
}
