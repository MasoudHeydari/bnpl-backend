package customercredituc

import (
	"github.com/parstis/bnpl-backend/pkg/core/model"
	// AssignInitialLimitInput carries the external Identity Service subject ID
	// (x-auth-subject-id / customer_profiles.identity_subject_id) to assign an initial limit for.
)

type AssignInitialLimitInput struct {
	// IdentitySubjectID is the Identity Service subject mapped to customer_profiles.identity_subject_id.
	IdentitySubjectID model.IdentitySubjectID
}
