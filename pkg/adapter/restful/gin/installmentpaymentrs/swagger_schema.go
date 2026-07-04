package installmentpaymentrs

import "github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"

// CreatePaymentAttemptRequestSchema is a schema-only payment-attempt request body.
type CreatePaymentAttemptRequestSchema createPaymentAttemptRequest

// PaymentAttemptSchema is a schema-only payment attempt payload.
type PaymentAttemptSchema paymentAttemptResponse

// CreatePaymentAttemptResponseSchema is a schema-only payment-attempt response data.
type CreatePaymentAttemptResponseSchema createPaymentAttemptResponse

// CreatePaymentAttemptSuccessSchema is a schema-only success envelope for installment pay.
type CreatePaymentAttemptSuccessSchema struct {
	Data CreatePaymentAttemptResponseSchema `json:"data"`
	Meta serdser.SuccessMetaNoPagination    `json:"meta"`
}
