package financialcorepaymentresultuc

import (
	"testing"

	testifymock "github.com/stretchr/testify/mock"

	"github.com/parstis/bnpl-backend/pkg/core/mock"
)

// newTestObserver returns a FinancialCorePaymentResultObserver that accepts any checkpoint.
func newTestObserver(t *testing.T) *mock.FinancialCorePaymentResultObserver {
	t.Helper()

	anything := testifymock.Anything
	observer := mock.NewFinancialCorePaymentResultObserver(t)
	observer.EXPECT().ResultReceived(anything, anything, anything, anything).Maybe()
	observer.EXPECT().ResultValidated(anything, anything, anything, anything).Maybe()
	observer.EXPECT().ResultValidationFailed(anything, anything, anything, anything).Maybe()
	observer.EXPECT().InboxDuplicateDetected(anything, anything, anything, anything).Maybe()
	observer.EXPECT().ApplicationStarted(anything, anything, anything).Maybe()
	observer.EXPECT().ApplicationSucceeded(anything, anything, anything).Maybe()
	observer.EXPECT().ApplicationAlreadyApplied(anything, anything, anything).Maybe()
	observer.EXPECT().ApplicationConflict(anything, anything, anything, anything).Maybe()
	observer.EXPECT().TerminalFailure(anything, anything, anything, anything).Maybe()
	observer.EXPECT().ContractActivated(anything, anything, anything).Maybe()
	observer.EXPECT().InstallmentRepaymentApplied(anything, anything, anything, anything).Maybe()
	observer.EXPECT().InstallmentRepaymentFailed(anything, anything, anything, anything, anything).Maybe()

	return observer
}
