//go:build e2e

package e2e_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/observer"
	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/financialcorepaymentreconciliationuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/installmentrepaymentuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/paymentattemptexpirationuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/paymentattemptreconciliationuc"
)

func mustInstallmentRepaymentObserver(t *testing.T, logger corelog.Logger) installmentrepaymentuc.Observer {
	t.Helper()

	o, err := observer.NewInstallmentRepayment(logger)
	require.NoError(t, err)

	return o
}

func mustFinancialCorePaymentReconciliationObserver(
	t *testing.T,
	logger corelog.Logger,
) financialcorepaymentreconciliationuc.Observer {
	t.Helper()

	o, err := observer.NewFinancialCorePaymentReconciliation(logger)
	require.NoError(t, err)

	return o
}

func mustPaymentAttemptReconciliationObserver(
	t *testing.T,
	logger corelog.Logger,
) paymentattemptreconciliationuc.Observer {
	t.Helper()

	o, err := observer.NewPaymentAttemptReconciliation(logger)
	require.NoError(t, err)

	return o
}

func mustPaymentAttemptExpirationObserver(
	t *testing.T,
	logger corelog.Logger,
) paymentattemptexpirationuc.Observer {
	t.Helper()

	o, err := observer.NewPaymentAttemptExpiration(logger)
	require.NoError(t, err)

	return o
}
