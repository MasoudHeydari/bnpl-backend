package customerrs

import "github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"

// InstallmentSchema is a schema-only unpaid-installment list item.
type UnpaidInstallmentSchema unpaidInstallmentResponse

// UnpaidInstallmentsListSuccessSchema is a schema-only success envelope for listing unpaid installments.
//
// Data is the installment array returned by SerKeyset. Meta.pagination carries
// keyset pagination state when present.
type UnpaidInstallmentsListSuccessSchema struct {
	Data []UnpaidInstallmentSchema `json:"data"`
	Meta serdser.SuccessMetaKeyset `json:"meta"`
}
