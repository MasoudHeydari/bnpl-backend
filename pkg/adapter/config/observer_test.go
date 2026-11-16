package config

import (
	"testing"

	testifymock "github.com/stretchr/testify/mock"

	"github.com/parstis/bnpl-backend/pkg/core/mock"
)

func newTestOutboxPublisherObserver(t *testing.T) *mock.OutboxPublisherObserver {
	t.Helper()

	observer := mock.NewOutboxPublisherObserver(t)
	observer.EXPECT().MessagePublished(testifymock.Anything, testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().MessageFailed(
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
	).Maybe()
	observer.EXPECT().MessageRetryScheduled(
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
	).Maybe()
	observer.EXPECT().MessageSkipped(
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
	).Maybe()
	observer.EXPECT().LeaseLost(testifymock.Anything, testifymock.Anything, testifymock.Anything).Maybe()

	return observer
}

func newTestCustomerCreditObserver(t *testing.T) *mock.CustomerCreditObserver {
	t.Helper()

	observer := mock.NewCustomerCreditObserver(t)
	observer.EXPECT().CustomerResolved(testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().AssessmentStarted(testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().AssessmentCompleted(testifymock.Anything, testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().LimitAssignmentAttempted(testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().LimitAssigned(testifymock.Anything, testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().LimitAlreadyAssigned(testifymock.Anything, testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().LimitAssignmentDenied(
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
	).Maybe()
	observer.EXPECT().AvailabilityRetrieved(testifymock.Anything, testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().UsageRestricted(testifymock.Anything, testifymock.Anything, testifymock.Anything).Maybe()

	return observer
}

func newTestDecisionObserver(t *testing.T) *mock.DecisionObserver {
	t.Helper()

	observer := mock.NewDecisionObserver(t)
	observer.EXPECT().EvaluationStarted(testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().AvailableCreditChecked(
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
	).Maybe()
	observer.EXPECT().DecisionApproved(testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().DecisionPartiallyApproved(testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().DecisionDenied(testifymock.Anything, testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().DecisionPersisted(testifymock.Anything, testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().DecisionReplayed(testifymock.Anything, testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().OfferExpired(testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().OfferInvalidated(testifymock.Anything, testifymock.Anything, testifymock.Anything).Maybe()

	return observer
}

func newTestContractObserver(t *testing.T) *mock.ContractObserver {
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
