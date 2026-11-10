package customercredituc

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	extcustomer "github.com/parstis/bnpl-backend/pkg/core/external/customer"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// resolveActiveExternalCustomerID canonicalizes the gateway mobile, resolves it
// through Customer Reference Service outside any DB transaction, and returns
// the CRS customer_id only when status is ACTIVE.
//
// That CRS id is persisted as customer_profiles.external_customer_id and is the
// subject key for initial-credit policy assignments (including trusted v2).
func (uc *UseCase) resolveActiveExternalCustomerID(
	ctx context.Context,
	mobileE164 string,
) (model.ExternalCustomerID, error) {
	const op = "customercredituc.UseCase.resolveActiveExternalCustomerID"

	mobile, err := normalizeAuthMobileE164(mobileE164)
	if err != nil {
		return model.ExternalCustomerID{}, err
	}

	resolved, err := uc.customerResolver.ResolveByMobile(
		ctx,
		extcustomer.ResolveByMobileInput{MobileE164: mobile},
	)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return model.ExternalCustomerID{}, customerReferenceNotFoundError(err)
		}

		return model.ExternalCustomerID{}, fmt.Errorf("%s: resolve customer by mobile: %w", op, err)
	}

	if resolved.ExternalCustomerID.IsZero() {
		return model.ExternalCustomerID{}, cerr.InternalError(
			fmt.Errorf("%s: customer reference service returned an empty customer id", op),
		)
	}

	switch resolved.Status {
	case extcustomer.ActiveStatus:
		return resolved.ExternalCustomerID, nil

	case extcustomer.SuspendedStatus:
		return model.ExternalCustomerID{}, customerReferenceNotActiveError(
			fmt.Errorf("%s: customer reference status is suspended", op),
		)

	default:
		return model.ExternalCustomerID{}, cerr.InternalError(
			fmt.Errorf("%s: unsupported customer reference status %q", op, resolved.Status),
		)
	}
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

	for _, r := range digits {
		if !unicode.IsDigit(r) {
			return "", authMobileNumberInvalidError()
		}
	}

	return mobile, nil
}
