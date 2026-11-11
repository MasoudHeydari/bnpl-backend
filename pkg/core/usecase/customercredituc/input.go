package customercredituc

import "github.com/parstis/bnpl-backend/pkg/core/model"

// AssignInitialLimitInput carries the authenticated Identity subject and mobile
// used to resolve the Customer Reference Service customer before assigning an
// initial credit limit.
type AssignInitialLimitInput struct {
	// SubjectID is the Identity Service subject from x-auth-subject-id
	// (customer_profiles.external_subject_id).
	SubjectID model.SubjectID

	// MobileE164 is the gateway-injected authenticated mobile
	// (x-auth-mobile-number ← JWT auth_mobile_number), canonical E.164.
	MobileE164 string
}
