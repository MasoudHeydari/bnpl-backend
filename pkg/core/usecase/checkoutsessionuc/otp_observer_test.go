package checkoutsessionuc

import (
	"testing"

	testifymock "github.com/stretchr/testify/mock"

	"github.com/parstis/bnpl-backend/pkg/core/mock"
)

// newTestOTPObserver returns an OTPObserver that accepts any checkpoint.
func newTestOTPObserver(t *testing.T) *mock.OTPObserver {
	t.Helper()

	anything := testifymock.Anything
	observer := mock.NewOTPObserver(t)
	observer.EXPECT().IssueRequested(anything, anything).Maybe()
	observer.EXPECT().IssueRateLimited(anything, anything, anything).Maybe()
	observer.EXPECT().ChallengeReused(anything, anything).Maybe()
	observer.EXPECT().ChallengeIssued(anything, anything).Maybe()
	observer.EXPECT().NotificationSubmitStarted(anything, anything, anything).Maybe()
	observer.EXPECT().ProviderAccepted(anything, anything, anything).Maybe()
	observer.EXPECT().ProviderRejected(anything, anything, anything).Maybe()
	observer.EXPECT().ProviderTimedOut(anything, anything).Maybe()
	observer.EXPECT().VerificationAttempted(anything, anything).Maybe()
	observer.EXPECT().VerificationSucceeded(anything, anything).Maybe()
	observer.EXPECT().VerificationFailed(anything, anything, anything).Maybe()
	observer.EXPECT().ChallengeExpired(anything, anything).Maybe()
	observer.EXPECT().ChallengeLocked(anything, anything).Maybe()

	return observer
}
