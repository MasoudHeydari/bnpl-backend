package identity

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// Resolver maps a mobile number to an Identity Service subject.
//
// Implementations perform network I/O and must never be called inside a
// database transaction.
type Resolver interface {
	// ResolveByMobile returns the Identity subject that owns mobileE164.
	//
	// BNPL calls this during External Partner Create with the merchant-asserted
	// customer mobile so eligibility and the Checkout Policy can be evaluated
	// before Hosted OTP. OTP confirmation later proves possession of the phone
	// and authorizes the prepared checkout generation; Identity resolution
	// itself does not prove OTP possession.
	//
	// An unknown mobile is reported as a cerr resource-not-found error.
	ResolveByMobile(ctx context.Context, input ResolveByMobileInput) (ResolveByMobileOutput, error)
}

// ResolveByMobileInput identifies the mobile to resolve.
type ResolveByMobileInput struct {
	// MobileE164 is the canonical E.164 mobile, for example +989121234567.
	MobileE164 string
}

// ResolveByMobileOutput carries the resolved Identity Service subject.
type ResolveByMobileOutput struct {
	// IdentitySubjectID is the external Identity Service subject
	// (customer_profiles.identity_subject_id), never the internal CustomerID.
	IdentitySubjectID model.IdentitySubjectID

	// SubjectType is the Identity wire subject type.
	SubjectType SubjectType

	// SubjectStatus is the Identity wire subject status.
	SubjectStatus SubjectStatus
}
