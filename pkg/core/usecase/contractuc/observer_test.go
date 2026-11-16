package contractuc

import (
	"testing"

	testifymock "github.com/stretchr/testify/mock"

	"github.com/parstis/bnpl-backend/pkg/core/mock"
)

// newTestObserver returns a ContractObserver that accepts any checkpoint.
func newTestObserver(t *testing.T) *mock.ContractObserver {
	t.Helper()

	observer := mock.NewContractObserver(t)
	observer.EXPECT().PreparationStarted(testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().PaymentPendingCreated(testifymock.Anything, testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().ContractReplayed(testifymock.Anything, testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().PaymentAttemptCreated(testifymock.Anything, testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().FinancialCoreRequestStarted(testifymock.Anything, testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().FinancialCoreResponseReceived(testifymock.Anything, testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().FinancialCoreFailed(
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
	).Maybe()
	observer.EXPECT().PaymentURLObtained(testifymock.Anything, testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().ContractActivated(testifymock.Anything, testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().CancellationSagaStarted(testifymock.Anything, testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().CancellationSagaCompleted(testifymock.Anything, testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().CancellationSagaFailed(
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
	).Maybe()
	observer.EXPECT().ContractCanceled(testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().ContractExpired(testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().PaymentPendingExpiryCompleted(testifymock.Anything, testifymock.Anything, testifymock.Anything).Maybe()

	return observer
}
