package prometheus

import (
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMetricsIncrementsAndExposition(t *testing.T) {
	t.Parallel()

	m := New()
	ctx := context.Background()

	m.CheckoutDecisionCreated(ctx, "approved")
	m.CheckoutDecisionCreated(ctx, "approved")
	m.ContractCreated(ctx, "payment_pending")
	m.PaymentAttemptCompleted(ctx, "succeeded", "financial_core")
	m.InstallmentStateChanged(ctx, "scheduled", "due")
	m.InstallmentStateChanged(ctx, "", "due")
	m.WebhookDeliveryCompleted(ctx, "delivered")
	m.WebhookDeliveryCompleted(ctx, "destination_disabled")

	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	require.Equal(t, "text/plain; version=0.0.4; charset=utf-8", rec.Header().Get("Content-Type"))

	body, err := io.ReadAll(rec.Body)
	require.NoError(t, err)
	text := string(body)

	require.Contains(t, text, `bnpl_business_checkout_decisions_created_total{outcome="approved"} 2`)
	require.Contains(t, text, `bnpl_business_contracts_created_total{status="payment_pending"} 1`)
	require.Contains(t, text, `bnpl_business_payment_attempts_completed_total{result="succeeded",channel="financial_core"} 1`)
	require.Contains(t, text, `bnpl_business_installment_state_changes_total{from="scheduled",to="due"} 1`)
	require.Contains(t, text, `bnpl_business_installment_state_changes_total{from="unknown",to="due"} 1`)
	require.Contains(t, text, `bnpl_business_webhook_deliveries_total{outcome="delivered"} 1`)
	require.Contains(t, text, `bnpl_business_webhook_deliveries_total{outcome="destination_disabled"} 1`)
}

func TestOutboundHTTPMetricsUseBoundedLabels(t *testing.T) {
	t.Parallel()

	m := New()
	ctx := context.Background()

	m.OutboundHTTPInFlightChanged(ctx, 1)
	m.OutboundHTTPQueueDepthChanged(ctx, 1)
	m.OutboundHTTPQueueWaitObserved(ctx, 25*time.Millisecond)
	m.OutboundHTTPRequestCompleted(ctx, "GET", "2xx", 40*time.Millisecond)
	m.OutboundHTTPSaturated(ctx)

	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	text := rec.Body.String()

	require.Contains(t, text, "bnpl_infrastructure_outbound_http_in_flight 1")
	require.Contains(t, text, "bnpl_infrastructure_outbound_http_queue_depth 1")
	require.Contains(t, text, "bnpl_infrastructure_outbound_http_queue_wait_seconds_count 1")
	require.Contains(t, text, `bnpl_infrastructure_outbound_http_requests_total{method="GET",outcome="2xx"} 1`)
	require.Contains(t, text, "bnpl_infrastructure_outbound_http_saturated_total 1")
	require.NotContains(t, text, "url=")
	require.NotContains(t, text, "path=")
}

func TestMetricsConcurrentIncrements(t *testing.T) {
	t.Parallel()

	m := New()
	ctx := context.Background()

	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.CheckoutDecisionCreated(ctx, "approved")
		}()
	}

	wg.Wait()

	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	require.True(t, strings.Contains(rec.Body.String(), `{outcome="approved"} 50`))
}
