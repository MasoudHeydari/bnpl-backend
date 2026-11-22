package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCheckoutSessionStatus_Transitions(t *testing.T) {
	t.Parallel()

	require.True(t, OpenCheckoutSessionStatus.CanTransitionTo(ConfirmingCheckoutSessionStatus))
	require.True(t, OpenCheckoutSessionStatus.CanTransitionTo(CanceledCheckoutSessionStatus))
	require.True(t, ConfirmingCheckoutSessionStatus.CanTransitionTo(ProcessingCheckoutSessionStatus))
	require.True(t, ConfirmingCheckoutSessionStatus.CanTransitionTo(OpenCheckoutSessionStatus))
	require.True(t, ProcessingCheckoutSessionStatus.CanTransitionTo(OpenCheckoutSessionStatus))

	require.True(t, OpenCheckoutSessionStatus.CanTransitionTo(ProcessingCheckoutSessionStatus))
	require.True(t, OpenCheckoutSessionStatus.CanTransitionTo(VerifyPendingCheckoutSessionStatus))
	require.True(t, ConfirmingCheckoutSessionStatus.CanTransitionTo(VerifyPendingCheckoutSessionStatus))
	require.True(t, ProcessingCheckoutSessionStatus.CanTransitionTo(VerifyPendingCheckoutSessionStatus))
	require.True(t, VerifyPendingCheckoutSessionStatus.CanTransitionTo(VerifiedByMerchantCheckoutSessionStatus))
	require.True(t, VerifyPendingCheckoutSessionStatus.CanTransitionTo(CancellationPendingCheckoutSessionStatus))
	require.True(t, CancellationPendingCheckoutSessionStatus.CanTransitionTo(CanceledCheckoutSessionStatus))

	require.False(t, VerifiedByMerchantCheckoutSessionStatus.CanTransitionTo(ProcessingCheckoutSessionStatus))
	require.False(t, CancellationPendingCheckoutSessionStatus.CanTransitionTo(VerifiedByMerchantCheckoutSessionStatus))
	require.False(t, CanceledCheckoutSessionStatus.CanTransitionTo(OpenCheckoutSessionStatus))
	require.False(t, ExpiredCheckoutSessionStatus.CanTransitionTo(OpenCheckoutSessionStatus))
	require.False(t, UnavailableCheckoutSessionStatus.CanTransitionTo(OpenCheckoutSessionStatus))

	require.True(t, VerifiedByMerchantCheckoutSessionStatus.IsTerminal())
	require.False(t, CancellationPendingCheckoutSessionStatus.IsTerminal())
	require.False(t, VerifyPendingCheckoutSessionStatus.IsTerminal())
	require.False(t, ProcessingCheckoutSessionStatus.IsTerminal())
	require.False(t, CheckoutSessionStatus("bogus").IsValid())
	require.False(t, CheckoutSessionStatus("completed").IsValid())
}

func TestCheckoutSessionStatus_AllowsCustomerContinuation(t *testing.T) {
	t.Parallel()

	require.True(t, OpenCheckoutSessionStatus.AllowsCustomerContinuation())

	require.False(t, UnavailableCheckoutSessionStatus.AllowsCustomerContinuation())
	require.False(t, ConfirmingCheckoutSessionStatus.AllowsCustomerContinuation())
	require.False(t, ProcessingCheckoutSessionStatus.AllowsCustomerContinuation())
	require.False(t, VerifyPendingCheckoutSessionStatus.AllowsCustomerContinuation())
	require.False(t, VerifiedByMerchantCheckoutSessionStatus.AllowsCustomerContinuation())
	require.False(t, CancellationPendingCheckoutSessionStatus.AllowsCustomerContinuation())
	require.False(t, CanceledCheckoutSessionStatus.AllowsCustomerContinuation())
	require.False(t, ExpiredCheckoutSessionStatus.AllowsCustomerContinuation())
}

func TestCheckoutSessionStatus_ActiveAndFinancialPredicates(t *testing.T) {
	t.Parallel()

	require.True(t, VerifyPendingCheckoutSessionStatus.IsActiveCheckoutAttempt())
	require.True(t, CancellationPendingCheckoutSessionStatus.IsActiveCheckoutAttempt())
	require.False(t, VerifiedByMerchantCheckoutSessionStatus.IsActiveCheckoutAttempt())

	require.True(t, VerifyPendingCheckoutSessionStatus.BlocksCancelAndExpiry())
	require.True(t, CancellationPendingCheckoutSessionStatus.BlocksCancelAndExpiry())

	require.True(t, VerifyPendingCheckoutSessionStatus.IsFinanciallySuccessful())
	require.True(t, VerifiedByMerchantCheckoutSessionStatus.IsFinanciallySuccessful())
	require.True(t, CancellationPendingCheckoutSessionStatus.IsFinanciallySuccessful())
	require.False(t, ProcessingCheckoutSessionStatus.IsFinanciallySuccessful())

	require.True(t, VerifiedByMerchantCheckoutSessionStatus.IsMerchantVerified())
	require.False(t, VerifyPendingCheckoutSessionStatus.IsMerchantVerified())
}

func TestCheckoutSession_IsExpiredAt(t *testing.T) {
	t.Parallel()

	expiresAt := time.Date(
		2026,
		time.August,
		9,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	session := CheckoutSession{ExpiresAt: expiresAt}

	require.False(t, session.IsExpiredAt(expiresAt.Add(-time.Second)))
	require.True(t, session.IsExpiredAt(expiresAt))
	require.True(t, session.IsExpiredAt(expiresAt.Add(time.Second)))
}

func TestCheckoutSession_MatchesCreateRequest(t *testing.T) {
	t.Parallel()

	storeID, err := ParseExternalOnlineStoreID("8e1a2b3c-4d5e-4f6a-8b7c-9d0e1f2a3b4c")
	require.NoError(t, err)

	session := CheckoutSession{
		ExternalOnlineStoreID: storeID,
		MerchantOrderID:       "order-1",
		CustomerReference:     "cust-1",
		ReturnDestination:     "storefront",
		MerchantState:         "state-1",
		RequestedAmountMinor:  1000,
		Currency:              IRRCurrency,
	}

	require.True(t, session.MatchesCreateRequest(
		storeID,
		"order-1",
		"cust-1",
		"storefront",
		"state-1",
		1000,
		IRRCurrency,
	))
	require.False(t, session.MatchesCreateRequest(
		storeID,
		"order-2",
		"cust-1",
		"storefront",
		"state-1",
		1000,
		IRRCurrency,
	))
}
