package customer

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// Resolver maps a mobile number to a Customer Reference Service customer.
//
// Implementations perform network I/O and must never be called inside a
// database transaction.
type Resolver interface {
	// ResolveByMobile returns the Customer Reference Service customer that
	// owns mobileE164.
	//
	// BNPL calls this during Partner Create (on_create) or customer Start
	// (on_start) with the merchant-asserted customer mobile so eligibility and
	// the Checkout Policy can be evaluated before Hosted OTP. OTP confirmation
	// later proves possession of the phone and authorizes the prepared
	// checkout generation; Customer Reference resolution itself does not prove
	// OTP possession.
	//
	// An unknown mobile is reported as a cerr resource-not-found error.
	ResolveByMobile(ctx context.Context, input ResolveByMobileInput) (ResolveByMobileOutput, error)
}

// ResolveByMobileInput identifies the mobile to resolve.
type ResolveByMobileInput struct {
	// MobileE164 is the canonical E.164 mobile, for example +989121234567.
	MobileE164 string
}

// ResolveByMobileOutput carries the resolved Customer Reference Service
// customer.
type ResolveByMobileOutput struct {
	// ExternalCustomerID is the Customer Reference Service customer_id.
	// Persist it as customer_profiles.identity_subject_id (model.IdentitySubjectID).
	// Never treat this as the BNPL-local model.CustomerID.
	ExternalCustomerID model.IdentitySubjectID

	// Status is the Customer Reference Service wire status.
	Status Status
}
