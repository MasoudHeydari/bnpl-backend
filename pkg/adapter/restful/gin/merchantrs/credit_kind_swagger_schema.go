package merchantrs

import "github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"

// SetCreditKindRequestSchema is a schema-only request body for enabling or disabling a credit kind.
type SetCreditKindRequestSchema struct {
	Enabled bool `json:"enabled" example:"true"`
}

// ListCreditKindsSchema is a schema-only merchant credit kind eligibility payload.
type ListCreditKindsSchema listCreditKindsResponse

// ListCreditKindsSuccessSchema is a schema-only success envelope for credit kind eligibility.
type ListCreditKindsSuccessSchema struct {
	Data ListCreditKindsSchema        `json:"data"`
	Meta serdser.SuccessMetaNoPagination `json:"meta"`
}

// SetCreditKindSchema is a schema-only payload for a credit kind eligibility change.
type SetCreditKindSchema setCreditKindResponse

// SetCreditKindSuccessSchema is a schema-only success envelope for a credit kind eligibility change.
type SetCreditKindSuccessSchema struct {
	Data SetCreditKindSchema          `json:"data"`
	Meta serdser.SuccessMetaNoPagination `json:"meta"`
}
