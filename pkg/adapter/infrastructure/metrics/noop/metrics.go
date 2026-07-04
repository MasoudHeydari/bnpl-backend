package noop

import (
	"context"
	"time"

	coremetrics "github.com/parstis/bnpl-backend/pkg/core/metrics"
)

// BusinessMetrics discards both business and infrastructure metric observations.
//
// Use this provider in tests and environments where metric collection is
// intentionally disabled. It owns no resources and intentionally exposes no
// Prometheus HTTP handler.
type BusinessMetrics struct{}

// New returns a no-op implementation of the complete runtime metrics contract.
func New() coremetrics.RuntimeMetrics { return BusinessMetrics{} }

// CheckoutDecisionCreated intentionally discards the observation.
func (BusinessMetrics) CheckoutDecisionCreated(context.Context, string) {}

// ContractCreated intentionally discards the observation.
func (BusinessMetrics) ContractCreated(context.Context, string) {}

// PaymentAttemptCompleted intentionally discards the observation.
func (BusinessMetrics) PaymentAttemptCompleted(context.Context, string, string) {}

// InstallmentStateChanged intentionally discards the observation.
func (BusinessMetrics) InstallmentStateChanged(context.Context, string, string) {}

// WebhookDeliveryCompleted intentionally discards the observation.
func (BusinessMetrics) WebhookDeliveryCompleted(context.Context, string) {}

// OutboundHTTPInFlightChanged intentionally discards the gauge delta.
func (BusinessMetrics) OutboundHTTPInFlightChanged(context.Context, int) {}

// OutboundHTTPQueueDepthChanged intentionally discards the gauge delta.
func (BusinessMetrics) OutboundHTTPQueueDepthChanged(context.Context, int) {}

// OutboundHTTPQueueWaitObserved intentionally discards the queue wait sample.
func (BusinessMetrics) OutboundHTTPQueueWaitObserved(context.Context, time.Duration) {}

// OutboundHTTPRequestCompleted intentionally discards the request observation.
func (BusinessMetrics) OutboundHTTPRequestCompleted(context.Context, string, string, time.Duration) {}

// OutboundHTTPSaturated intentionally discards the saturation observation.
func (BusinessMetrics) OutboundHTTPSaturated(context.Context) {}

var _ coremetrics.RuntimeMetrics = BusinessMetrics{}
