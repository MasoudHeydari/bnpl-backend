package customercredituc_test

import (
	"testing"

	testifymock "github.com/stretchr/testify/mock"

	"github.com/parstis/bnpl-backend/pkg/core/mock"
)

// newTestObserver returns a CustomerCreditObserver that accepts any checkpoint.
func newTestObserver(t *testing.T) *mock.CustomerCreditObserver {
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
