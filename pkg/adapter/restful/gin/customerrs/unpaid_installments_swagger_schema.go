package customerrs

import "github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"

// UnpaidInstallmentSchema is a schema-only unpaid-installment list item.
type UnpaidInstallmentSchema unpaidInstallmentResponse

// UnpaidInstallmentsListDataSchema is the structured data payload for listing
// unpaid installments together with the customer-wide repayment summary.
type UnpaidInstallmentsListDataSchema unpaidInstallmentsListData

// UnpaidInstallmentsListSuccessSchema is a schema-only success envelope for listing unpaid installments.
//
// Data carries both the current installment page and the customer-wide
// repayment summary. Meta.pagination carries keyset pagination state.
type UnpaidInstallmentsListSuccessSchema struct {
	Data UnpaidInstallmentsListDataSchema `json:"data"`
	Meta serdser.SuccessMetaKeyset        `json:"meta"`
}
