package decisionuc_test

import (
	"testing"

	testifymock "github.com/stretchr/testify/mock"

	"github.com/parstis/bnpl-backend/pkg/core/mock"
)

// newTestObserver returns a DecisionObserver that accepts any checkpoint.
func newTestObserver(t *testing.T) *mock.DecisionObserver {
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
