//go:build e2e

package e2e_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/checkoutsessionuc"
)

func TestE2E_ConfirmationStatus_NotStartedBeforeConfirm(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{externalCustomerID: externalCustomerID})
	seed := seedMerchantFundingCheckout(t, h, externalCustomerID)

	got, err := h.CheckoutSession.GetExternalConfirmationStatus(h.ctx, checkoutsessionuc.GetExternalConfirmationStatusInput{
		Token: seed.Session.PublicToken,
	})
	require.NoError(t, err)
	require.Equal(t, model.OpenCheckoutSessionStatus, got.Status)
	require.Equal(t, checkoutsessionuc.ConfirmationStatusNotStarted, got.ConfirmationStatus)
	require.Equal(t, checkoutsessionuc.ConfirmationNextActionStartConfirm, got.NextAction)
	require.Equal(t, seed.Session.PublicToken, got.Token)
}

func TestE2E_ConfirmationStatus_RejectsMalformedAndUnknownToken(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{externalCustomerID: externalCustomerID})

	_, err := h.CheckoutSession.GetExternalConfirmationStatus(h.ctx, checkoutsessionuc.GetExternalConfirmationStatusInput{
		Token: "not-a-token",
	})
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeInvalidArgument))

	unknown, parseErr := model.ParseCheckoutSessionToken(
		"cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	)
	require.NoError(t, parseErr)

	_, err = h.CheckoutSession.GetExternalConfirmationStatus(h.ctx, checkoutsessionuc.GetExternalConfirmationStatusInput{
		Token: unknown,
	})
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeResourceNotFound))
}

func TestE2E_ConfirmationStatus_InternalCheckoutNotFound(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{externalCustomerID: externalCustomerID})
	seed := seedMerchantFundingCheckout(t, h, externalCustomerID)

	_, err := testDB.Exec(h.ctx, `
		UPDATE bnpl.checkout_sessions
		SET origin = $2
		WHERE id = $1
	`, seed.Session.ID, model.InternalCheckoutSessionOrigin)
	require.NoError(t, err)

	_, err = h.CheckoutSession.GetExternalConfirmationStatus(h.ctx, checkoutsessionuc.GetExternalConfirmationStatusInput{
		Token: seed.Session.PublicToken,
	})
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeResourceNotFound))
}

func TestE2E_ConfirmationStatus_CompletedAfterMerchantFunding(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{externalCustomerID: externalCustomerID})
	seed := seedMerchantFundingCheckout(t, h, externalCustomerID)

	out, err := h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	require.NoError(t, err)
	require.Equal(t, model.CompletedCheckoutSessionStatus, out.Session.Status)

	beforeFCCalls := len(h.fc.callRecords())
	beforeOutbox := h.countIntegrationOutboxRows()

	got, err := h.CheckoutSession.GetExternalConfirmationStatus(h.ctx, checkoutsessionuc.GetExternalConfirmationStatusInput{
		Token: seed.Session.PublicToken,
	})
	require.NoError(t, err)
	require.Equal(t, checkoutsessionuc.ConfirmationStatusCompleted, got.ConfirmationStatus)
	require.Equal(t, checkoutsessionuc.ConfirmationNextActionShowSuccess, got.NextAction)

	// Repeated polling must not mutate durable state or call Financial Core.
	got2, err := h.CheckoutSession.GetExternalConfirmationStatus(h.ctx, checkoutsessionuc.GetExternalConfirmationStatusInput{
		Token: seed.Session.PublicToken,
	})
	require.NoError(t, err)
	require.Equal(t, got, got2)
	require.Equal(t, beforeOutbox, h.countIntegrationOutboxRows())
	require.Equal(t, beforeFCCalls, len(h.fc.callRecords()))
}

func TestE2E_ConfirmationStatus_PaymentActionRequiredWithoutURL(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{externalCustomerID: externalCustomerID})
	seed := seedInitialPaymentCheckout(t, h, externalCustomerID)

	out, err := h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	require.NoError(t, err)
	require.True(t, out.PaymentRequired)
	require.NotNil(t, out.Session.ContractID)

	got, err := h.CheckoutSession.GetExternalConfirmationStatus(h.ctx, checkoutsessionuc.GetExternalConfirmationStatusInput{
		Token: seed.Session.PublicToken,
	})
	require.NoError(t, err)
	require.Equal(t, checkoutsessionuc.ConfirmationStatusPaymentActionRequired, got.ConfirmationStatus)
	require.Equal(t, checkoutsessionuc.ConfirmationNextActionResumePayment, got.NextAction)
	require.Equal(t, seed.Session.PublicToken, got.Token)
}

func TestE2E_ConfirmationStatus_RetryAvailableAfterTerminalFailure(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{externalCustomerID: externalCustomerID})
	seed := seedInitialPaymentCheckout(t, h, externalCustomerID)

	out, err := h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	require.NoError(t, err)

	attempt := h.queryCheckoutPaymentAttempt(*out.Session.ContractID)
	contractRow := h.queryContractBySession(out.Session)
	failure := h.buildCheckoutPaymentFailureProcessInput(attempt, contractRow, "fc-event-status-retry")
	h.processPaymentResult(failure)

	got, err := h.CheckoutSession.GetExternalConfirmationStatus(h.ctx, checkoutsessionuc.GetExternalConfirmationStatusInput{
		Token: seed.Session.PublicToken,
	})
	require.NoError(t, err)
	require.Equal(t, model.OpenCheckoutSessionStatus, got.Status)
	require.Equal(t, checkoutsessionuc.ConfirmationStatusRetryAvailable, got.ConfirmationStatus)
	require.Equal(t, checkoutsessionuc.ConfirmationNextActionRetryPayment, got.NextAction)
}
