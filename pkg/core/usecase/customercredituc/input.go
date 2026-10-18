package customercredituc

import (
	"github.com/parstis/bnpl-backend/pkg/core/model"
	// AssignInitialLimitInput carries the external Identity Service subject ID
	// (x-auth-subject-id / customer_profiles.external_customer_id) to assign an initial limit for.
)

type AssignInitialLimitInput struct {
	// ExternalCustomerID is the Identity Service subject mapped to customer_profiles.external_customer_id.
	ExternalCustomerID model.ExternalCustomerID
}
