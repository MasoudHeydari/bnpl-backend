package checkoutsessionuc

import (
	"testing"

	testifymock "github.com/stretchr/testify/mock"

	"github.com/parstis/bnpl-backend/pkg/core/mock"
)

// newAllowAllCheckoutObserver returns a Mockery Observer with Maybe expectations
// for every method so existing harness tests do not need per-call wiring.
func newAllowAllCheckoutObserver(t *testing.T) *mock.CheckoutSessionObserver {
	t.Helper()

	observer := mock.NewCheckoutSessionObserver(t)
	observer.EXPECT().SessionCreateRequested(testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().SessionInputRejected(testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().MerchantValidated(testifymock.Anything, testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().SessionCreated(testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().SessionReplayed(testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().SessionRetrieved(testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().CustomerResolutionStarted(testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().CustomerResolved(testifymock.Anything, testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().CustomerResolutionFailed(testifymock.Anything, testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().OfferPreparationStarted(testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().OfferPrepared(testifymock.Anything, testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().OfferPreparationFailed(testifymock.Anything, testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().SessionStarted(testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().SessionStartReplayed(testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().ConfirmRequested(testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().ConfirmSagaStarted(testifymock.Anything, testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().ConfirmSagaStepStarted(testifymock.Anything, testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().ConfirmSagaStepCompleted(testifymock.Anything, testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().ConfirmSagaFailed(testifymock.Anything, testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().ConfirmSagaCompensating(testifymock.Anything, testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().SessionCanceled(testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().SessionExpired(testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().HostedHandoffPrepared(testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().ExpirySweepCompleted(testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().ConfirmingRecoveryCompleted(testifymock.Anything, testifymock.Anything).Maybe()

	return observer
}
