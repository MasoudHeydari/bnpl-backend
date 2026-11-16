//go:build integration && development

package collectionsuc

import (
	"testing"

	"github.com/stretchr/testify/require"

	lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/observer"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/financialcorepaymentresultuc"
)

func mustPaymentResultObserver(t *testing.T) financialcorepaymentresultuc.Observer {
	t.Helper()

	o, err := observer.NewFinancialCorePaymentResult(lognoop.New())
	require.NoError(t, err)

	return o
}
