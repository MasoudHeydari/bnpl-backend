package customerrs

import "github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"

// PaymentSchema is a schema-only installment payment history item.
type PaymentSchema paymentResponse

// PaymentsListSuccessSchema is a schema-only success envelope for listing installment payments.
//
// Data is the payment array returned by SerKeyset. Meta.pagination carries
// keyset pagination state when present.
type PaymentsListSuccessSchema struct {
	Data []PaymentSchema           `json:"data"`
	Meta serdser.SuccessMetaKeyset `json:"meta"`
}
