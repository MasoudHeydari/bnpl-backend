package merchantrs

import "github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"

// SetCreditProductRequestSchema is a schema-only request body for enabling or disabling a credit product.
type SetCreditProductRequestSchema struct {
	Enabled bool `json:"enabled" example:"true"`
}

// ListCreditProductsSchema is a schema-only merchant credit product eligibility payload.
type ListCreditProductsSchema listCreditProductsResponse

// ListCreditProductsSuccessSchema is a schema-only success envelope for credit product eligibility.
type ListCreditProductsSuccessSchema struct {
	Data ListCreditProductsSchema        `json:"data"`
	Meta serdser.SuccessMetaNoPagination `json:"meta"`
}

// SetCreditProductSchema is a schema-only payload for a credit product eligibility change.
type SetCreditProductSchema setCreditProductResponse

// SetCreditProductSuccessSchema is a schema-only success envelope for a credit product eligibility change.
type SetCreditProductSuccessSchema struct {
	Data SetCreditProductSchema          `json:"data"`
	Meta serdser.SuccessMetaNoPagination `json:"meta"`
}
