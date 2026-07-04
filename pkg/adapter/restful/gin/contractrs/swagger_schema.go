package contractrs

import "github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"

// CreateContractRequestSchema is a schema-only empty create-contract body.
// Public callers never select a payment provider.
type CreateContractRequestSchema struct{}

// ContractSchema is a schema-only contract payload.
type ContractSchema contractResponse

// CreateContractResponseSchema is a schema-only create-contract response data.
type CreateContractResponseSchema createContractResponse

// CreateContractSuccessSchema is a schema-only success envelope for contract create.
type CreateContractSuccessSchema struct {
	Data CreateContractResponseSchema    `json:"data"`
	Meta serdser.SuccessMetaNoPagination `json:"meta"`
}

// CancelRequestSchema is a schema-only contract cancellation request body.
type CancelRequestSchema cancelRequest

// CancelResponseSchema is a schema-only contract cancellation response data.
type CancelResponseSchema cancelResponse

// CancelSuccessSchema is a schema-only success envelope for contract cancellation.
type CancelSuccessSchema struct {
	Data CancelResponseSchema            `json:"data"`
	Meta serdser.SuccessMetaNoPagination `json:"meta"`
}
