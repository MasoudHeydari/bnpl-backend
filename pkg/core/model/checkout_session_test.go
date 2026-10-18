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

	// A linked checkout whose payment attempt was replaced re-enters processing,
	// and authoritative success can complete that open session if the processing
	// update was lost. The first Confirm of an unlinked session still claims
	// confirming before any contract exists.
	require.True(t, OpenCheckoutSessionStatus.CanTransitionTo(ProcessingCheckoutSessionStatus))
	require.True(t, OpenCheckoutSessionStatus.CanTransitionTo(CompletedCheckoutSessionStatus))

	// Terminal states never reopen.
	require.False(t, CanceledCheckoutSessionStatus.CanTransitionTo(OpenCheckoutSessionStatus))
	require.False(t, CompletedCheckoutSessionStatus.CanTransitionTo(CanceledCheckoutSessionStatus))
	require.False(t, ExpiredCheckoutSessionStatus.CanTransitionTo(OpenCheckoutSessionStatus))
	require.False(t, UnavailableCheckoutSessionStatus.CanTransitionTo(OpenCheckoutSessionStatus))

	require.True(t, CompletedCheckoutSessionStatus.IsTerminal())
	require.False(t, ProcessingCheckoutSessionStatus.IsTerminal())
	require.False(t, CheckoutSessionStatus("bogus").IsValid())
}

func TestCheckoutSessionStatus_AllowsCustomerContinuation(t *testing.T) {
	t.Parallel()

	require.True(t, OpenCheckoutSessionStatus.AllowsCustomerContinuation())

	// A confirming claim or pending external payment outcome must not be re-evaluated locally.
	require.False(t, UnavailableCheckoutSessionStatus.AllowsCustomerContinuation())
	require.False(t, ConfirmingCheckoutSessionStatus.AllowsCustomerContinuation())
	require.False(t, ProcessingCheckoutSessionStatus.AllowsCustomerContinuation())
	require.False(t, CanceledCheckoutSessionStatus.AllowsCustomerContinuation())
	require.False(t, ExpiredCheckoutSessionStatus.AllowsCustomerContinuation())
}

func TestCheckoutSession_IsExpiredAt(t *testing.T) {
	t.Parallel()

	expiresAt := time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC)
	session := CheckoutSession{ExpiresAt: expiresAt}

	require.False(t, session.IsExpiredAt(expiresAt.Add(-time.Second)))
	require.True(t, session.IsExpiredAt(expiresAt))
	require.True(t, session.IsExpiredAt(expiresAt.Add(time.Second)))
}

func TestCheckoutSession_MatchesCreateRequest(t *testing.T) {
	t.Parallel()

	storeID, err := ParseOnlineStoreID("8e1a2b3c-4d5e-4f6a-8b7c-9d0e1f2a3b4c")
	require.NoError(t, err)

	session := CheckoutSession{
		OnlineStoreID:        storeID,
		MerchantOrderID:      "order-1",
		CustomerReference:    "cust-1",
		ReturnDestination:    "storefront",
		MerchantState:        "cart-42",
		RequestedAmountMinor: 150000000,
		Currency:             IRRCurrency,
	}

	require.True(t, session.MatchesCreateRequest(
		storeID, "order-1", "cust-1", "storefront", "cart-42", 150000000, IRRCurrency,
	))

	// A reused merchant order reference with different terms is a conflict.
	require.False(t, session.MatchesCreateRequest(
		storeID, "order-2", "cust-1", "storefront", "cart-42", 150000000, IRRCurrency,
	))
	require.False(t, session.MatchesCreateRequest(
		storeID, "order-1", "cust-1", "storefront", "cart-42", 150000001, IRRCurrency,
	))
	require.False(t, session.MatchesCreateRequest(
		storeID, "order-1", "cust-2", "storefront", "cart-42", 150000000, IRRCurrency,
	))
}
