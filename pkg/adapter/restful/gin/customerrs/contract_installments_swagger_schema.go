package customerrs

import "github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"

// InstallmentSchema is a schema-only installment schedule row.
type ContractInstallmentSchema contractInstallmentResponse

// NextPayableInstallmentSchema is a schema-only next-payable installment object.
type NextPayableInstallmentSchema nextPayableInstallmentResponse

// GetResponseSchema is a schema-only contract installment list payload.
type GetResponseSchema getResponse

// GetSuccessSchema is a schema-only success envelope for contract installment listing.
type GetSuccessSchema struct {
	Data GetResponseSchema               `json:"data"`
	Meta serdser.SuccessMetaNoPagination `json:"meta"`
}
