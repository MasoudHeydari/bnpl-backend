package merchantterminalrs

import "github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"

// RotateRequestSchema is a schema-only representation of the optional rotate body.
type RotateRequestSchema rotateRequest

// GetResponseSchema is a schema-only representation of one terminal in the get
// response data array.
type GetResponseSchema getResponse

// GetSuccessSchema is a schema-only success envelope for get.
//
// Data is a list: empty when the merchant has no terminal yet, or one entry
// when provisioned (merchants currently own at most one terminal).
type GetSuccessSchema struct {
	Data []GetResponseSchema             `json:"data"`
	Meta serdser.SuccessMetaNoPagination `json:"meta"`
}

// ProvisionResponseSchema is a schema-only representation of the provision
// response data.
type ProvisionResponseSchema provisionResponse

// ProvisionSuccessSchema is a schema-only success envelope for provisioning.
type ProvisionSuccessSchema struct {
	Data ProvisionResponseSchema         `json:"data"`
	Meta serdser.SuccessMetaNoPagination `json:"meta"`
}

// RotateResponseSchema is a schema-only representation of the rotate response data.
type RotateResponseSchema rotateResponse

// RotateSuccessSchema is a schema-only success envelope for rotation.
type RotateSuccessSchema struct {
	Data RotateResponseSchema            `json:"data"`
	Meta serdser.SuccessMetaNoPagination `json:"meta"`
}

// RetireResponseSchema is a schema-only representation of the retire response data.
type RetireResponseSchema retireResponse

// RetireSuccessSchema is a schema-only success envelope for retirement.
type RetireSuccessSchema struct {
	Data RetireResponseSchema            `json:"data"`
	Meta serdser.SuccessMetaNoPagination `json:"meta"`
}
