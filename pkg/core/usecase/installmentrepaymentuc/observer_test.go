package installmentrepaymentuc

import (
	"testing"

	testifymock "github.com/stretchr/testify/mock"

	"github.com/parstis/bnpl-backend/pkg/core/mock"
)

// newTestObserver returns an InstallmentRepaymentObserver that accepts any checkpoint.
func newTestObserver(t *testing.T) *mock.InstallmentRepaymentObserver {
	t.Helper()

	observer := mock.NewInstallmentRepaymentObserver(t)
	observer.EXPECT().RepaymentRequested(testifymock.Anything, testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().PaymentAttemptCreated(testifymock.Anything, testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().PaymentAttemptReplayed(testifymock.Anything, testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().FinancialCoreRequestStarted(testifymock.Anything, testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().FinancialCoreResponseReceived(testifymock.Anything, testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().FinancialCoreFailed(
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
	).Maybe()
	observer.EXPECT().PaymentURLObtained(testifymock.Anything, testifymock.Anything, testifymock.Anything).Maybe()

	return observer
}
