package jobs

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/mock"
)

func TestRegistry_Resolve_KnownHandlers(t *testing.T) {
	t.Parallel()

	logger := mock.NewLogger(t)
	registry := NewRegistry(
		logger,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	handlers := []string{
		"runtime_goroutine_metrics",
		"outbox_publisher",
		"installment_offer_expiry",
		"contract_payment_pending_expiry",
		"checkout_session_expiry",
		"checkout_session_confirming_recovery",
		"saga_executor",
		"installment_mark_due",
		"installment_mark_overdue",
		"payment_attempt_reconciliation",
		"merchant_terminal_issuance_scrub",
	}

	for _, handler := range handlers {
		job, err := registry.Resolve(handler)
		require.NoError(t, err)
		require.NotNil(t, job)
	}
}

func TestRegistry_Resolve_UnknownHandler(t *testing.T) {
	t.Parallel()

	registry := NewRegistry(
		mock.NewLogger(t),
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	_, err := registry.Resolve("unknown_handler")
	require.Error(t, err)
	require.ErrorContains(t, err, "unknown task handler")
}
