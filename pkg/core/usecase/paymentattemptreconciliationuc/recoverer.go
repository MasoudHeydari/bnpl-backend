package paymentattemptreconciliationuc

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/core/usecase/financialcorepaymentresultuc"
)

// PaymentStatusRecoverer recovers submitted-payment outcomes via Financial Core
// GetPaymentStatus. It replaces the former Kafka reconciliation request path.
type PaymentStatusRecoverer interface {
	RecoverByGetPaymentStatus(
		ctx context.Context,
		input financialcorepaymentresultuc.RecoverByGetPaymentStatusInput,
	) (financialcorepaymentresultuc.RecoverByGetPaymentStatusOutput, error)
}

// Ensure the Financial Core payment-result use case satisfies the recoverer.
var _ PaymentStatusRecoverer = (*financialcorepaymentresultuc.UseCase)(nil)
