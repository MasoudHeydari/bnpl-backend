//go:build integration && development

package collectionsuc

import (
	"context"

	lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/observer"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/financialcorepaymentreconciliationuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/financialcorepaymentresultuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/installmentrepaymentuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/paymentattemptexpirationuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/paymentattemptreconciliationuc"
)

type stubPaymentStatusRecoverer struct {
	calls []financialcorepaymentresultuc.RecoverByGetPaymentStatusInput
	err   error
}

func (s *stubPaymentStatusRecoverer) RecoverByGetPaymentStatus(
	_ context.Context,
	input financialcorepaymentresultuc.RecoverByGetPaymentStatusInput,
) (financialcorepaymentresultuc.RecoverByGetPaymentStatusOutput, error) {
	s.calls = append(s.calls, input)
	if s.err != nil {
		return financialcorepaymentresultuc.RecoverByGetPaymentStatusOutput{}, s.err
	}

	return financialcorepaymentresultuc.RecoverByGetPaymentStatusOutput{
		Result: financialcorepaymentresultuc.ProcessResultPaymentApplied,
	}, nil
}

func newStubPaymentStatusRecoverer() *stubPaymentStatusRecoverer {
	return &stubPaymentStatusRecoverer{}
}

// The flow observers below are the production adapter implementations over a
// logger that discards records. Constructors fail only for a nil logger, so a
// failure here is a test programming error.

func newInstallmentRepaymentObserver() installmentrepaymentuc.Observer {
	o, err := observer.NewInstallmentRepayment(lognoop.New())
	if err != nil {
		panic(err)
	}

	return o
}

func newFinancialCorePaymentReconciliationObserver() financialcorepaymentreconciliationuc.Observer {
	o, err := observer.NewFinancialCorePaymentReconciliation(lognoop.New())
	if err != nil {
		panic(err)
	}

	return o
}

func newPaymentAttemptReconciliationObserver() paymentattemptreconciliationuc.Observer {
	o, err := observer.NewPaymentAttemptReconciliation(lognoop.New())
	if err != nil {
		panic(err)
	}

	return o
}

func newPaymentAttemptExpirationObserver() paymentattemptexpirationuc.Observer {
	o, err := observer.NewPaymentAttemptExpiration(lognoop.New())
	if err != nil {
		panic(err)
	}

	return o
}
