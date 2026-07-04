package prometheus

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	coremetrics "github.com/parstis/bnpl-backend/pkg/core/metrics"
)

// Metrics is a small Prometheus text-format collector for bounded business
// and infrastructure metrics.
//
// It intentionally does not accept arbitrary labels. Callers must pass only
// low-cardinality enumerations (outcome, status, result, channel, HTTP method,
// HTTP outcome, and from/to state). URLs, entity IDs, idempotency keys, and
// trace identifiers must never appear as label values.
//
// Metrics are operational aggregates only. They are not a substitute for audit
// logging and are not a source of monetary truth — Financial Core remains the
// authority for payment and ledger state.
type Metrics struct {
	mu sync.RWMutex

	checkoutDecisions map[string]uint64
	contracts         map[string]uint64
	paymentAttempts   map[paymentAttemptLabels]uint64
	installmentStates map[installmentStateLabels]uint64
	webhookDeliveries map[string]uint64

	outboundHTTPInFlight   int64
	outboundHTTPQueueDepth int64
	outboundHTTPQueueWait  durationSummary
	outboundHTTPRequests   map[outboundHTTPLabels]uint64
	outboundHTTPDuration   map[outboundHTTPLabels]durationSummary
	outboundHTTPSaturated  uint64
}

// paymentAttemptLabels is the bounded label key for payment-attempt counters.
type paymentAttemptLabels struct {
	result  string
	channel string
}

// installmentStateLabels is the bounded label key for installment transitions.
type installmentStateLabels struct {
	from string
	to   string
}

// outboundHTTPLabels is the bounded label key for outbound HTTP observations.
// Raw URLs and identifiers are intentionally absent from this key.
type outboundHTTPLabels struct {
	method  string
	outcome string
}

// durationSummary stores the minimal sum/count state required by the emitted
// Prometheus summary-style series.
type durationSummary struct {
	count uint64
	sum   float64
}

// New constructs an isolated business metrics collector that also implements
// http.Handler for Prometheus text exposition.
func New() *Metrics {
	return &Metrics{
		checkoutDecisions:    make(map[string]uint64),
		contracts:            make(map[string]uint64),
		paymentAttempts:      make(map[paymentAttemptLabels]uint64),
		installmentStates:    make(map[installmentStateLabels]uint64),
		webhookDeliveries:    make(map[string]uint64),
		outboundHTTPRequests: make(map[outboundHTTPLabels]uint64),
		outboundHTTPDuration: make(map[outboundHTTPLabels]durationSummary),
	}
}

var _ coremetrics.RuntimeMetrics = (*Metrics)(nil)
var _ http.Handler = (*Metrics)(nil)

// CheckoutDecisionCreated increments bnpl_business_checkout_decisions_created_total.
//
// Call only after a newly created decision has been committed. Idempotent
// replays must not call this method.
func (m *Metrics) CheckoutDecisionCreated(_ context.Context, outcome string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.checkoutDecisions[normalizeLabel(outcome)]++
}

// ContractCreated increments bnpl_business_contracts_created_total.
//
// Call only after a newly created contract has been committed successfully for
// the end-to-end create path. Idempotent replays must not call this method.
func (m *Metrics) ContractCreated(_ context.Context, status string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.contracts[normalizeLabel(status)]++
}

// PaymentAttemptCompleted increments bnpl_business_payment_attempts_completed_total.
//
// Call only after a payment-attempt terminal transition has been durably
// applied. Inbox/idempotent replays that do not apply a new completion must not
// call this method.
func (m *Metrics) PaymentAttemptCompleted(_ context.Context, result, channel string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.paymentAttempts[paymentAttemptLabels{
		result:  normalizeLabel(result),
		channel: normalizeLabel(channel),
	}]++
}

// InstallmentStateChanged increments bnpl_business_installment_state_changes_total.
//
// Call once per committed lifecycle transition after the transaction succeeds.
func (m *Metrics) InstallmentStateChanged(_ context.Context, from, to string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.installmentStates[installmentStateLabels{
		from: normalizeLabel(from),
		to:   normalizeLabel(to),
	}]++
}

// WebhookDeliveryCompleted increments bnpl_business_webhook_deliveries_total.
//
// Call once per classified delivery attempt, including permanent skips such as
// a disabled destination.
func (m *Metrics) WebhookDeliveryCompleted(_ context.Context, outcome string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.webhookDeliveries[normalizeLabel(outcome)]++
}

// OutboundHTTPInFlightChanged updates the current in-flight outbound request gauge.
func (m *Metrics) OutboundHTTPInFlightChanged(_ context.Context, delta int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.outboundHTTPInFlight += int64(delta)
	if m.outboundHTTPInFlight < 0 {
		m.outboundHTTPInFlight = 0
	}
}

// OutboundHTTPQueueDepthChanged updates the current bounded-queue depth gauge.
func (m *Metrics) OutboundHTTPQueueDepthChanged(_ context.Context, delta int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.outboundHTTPQueueDepth += int64(delta)
	if m.outboundHTTPQueueDepth < 0 {
		m.outboundHTTPQueueDepth = 0
	}
}

// OutboundHTTPQueueWaitObserved records queue wait time without any unbounded labels.
func (m *Metrics) OutboundHTTPQueueWaitObserved(_ context.Context, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.outboundHTTPQueueWait.count++
	m.outboundHTTPQueueWait.sum += duration.Seconds()
}

// OutboundHTTPRequestCompleted records an outbound HTTP attempt using the bounded
// method/outcome labels produced by the HTTP infrastructure. URLs are
// intentionally absent to avoid high-cardinality time series.
func (m *Metrics) OutboundHTTPRequestCompleted(_ context.Context, method, outcome string, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	labels := outboundHTTPLabels{
		method:  normalizeLabel(method),
		outcome: normalizeLabel(outcome),
	}
	m.outboundHTTPRequests[labels]++
	summary := m.outboundHTTPDuration[labels]
	summary.count++
	summary.sum += duration.Seconds()
	m.outboundHTTPDuration[labels] = summary
}

// OutboundHTTPSaturated increments the bounded-capacity rejection counter.
func (m *Metrics) OutboundHTTPSaturated(_ context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.outboundHTTPSaturated++
}

// ServeHTTP writes the Prometheus text exposition format for scrapers.
//
// This handler is intended for an internal metrics listener only — do not mount
// it on the public application ingress.
func (m *Metrics) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	m.mu.RLock()
	defer m.mu.RUnlock()

	writeSingleLabelCounter(
		w,
		"bnpl_business_checkout_decisions_created_total",
		"Total number of newly persisted checkout decisions by outcome.",
		"outcome",
		m.checkoutDecisions,
	)
	writeSingleLabelCounter(
		w,
		"bnpl_business_contracts_created_total",
		"Total number of newly persisted contracts by initial status.",
		"status",
		m.contracts,
	)

	_, _ = io.WriteString(
		w,
		"# HELP bnpl_business_payment_attempts_completed_total Total number of durably completed payment attempts by result and channel.\n",
	)
	_, _ = io.WriteString(w, "# TYPE bnpl_business_payment_attempts_completed_total counter\n")
	paymentKeys := make([]paymentAttemptLabels, 0, len(m.paymentAttempts))
	for labels := range m.paymentAttempts {
		paymentKeys = append(paymentKeys, labels)
	}

	sort.Slice(paymentKeys, func(i, j int) bool {
		if paymentKeys[i].result == paymentKeys[j].result {
			return paymentKeys[i].channel < paymentKeys[j].channel
		}

		return paymentKeys[i].result < paymentKeys[j].result
	})
	for _, labels := range paymentKeys {
		_, _ = fmt.Fprintf(
			w,
			"bnpl_business_payment_attempts_completed_total{result=%s,channel=%s} %d\n",
			quoteLabel(labels.result),
			quoteLabel(labels.channel),
			m.paymentAttempts[labels],
		)
	}

	_, _ = io.WriteString(w, "# HELP bnpl_business_installment_state_changes_total Total number of committed installment lifecycle transitions.\n")
	_, _ = io.WriteString(w, "# TYPE bnpl_business_installment_state_changes_total counter\n")
	stateKeys := make([]installmentStateLabels, 0, len(m.installmentStates))
	for labels := range m.installmentStates {
		stateKeys = append(stateKeys, labels)
	}

	sort.Slice(stateKeys, func(i, j int) bool {
		if stateKeys[i].from == stateKeys[j].from {
			return stateKeys[i].to < stateKeys[j].to
		}

		return stateKeys[i].from < stateKeys[j].from
	})
	for _, labels := range stateKeys {
		_, _ = fmt.Fprintf(
			w,
			"bnpl_business_installment_state_changes_total{from=%s,to=%s} %d\n",
			quoteLabel(labels.from),
			quoteLabel(labels.to),
			m.installmentStates[labels],
		)
	}

	_, _ = io.WriteString(w, "# HELP bnpl_business_webhook_deliveries_total Total merchant webhook delivery attempts by classified outcome.\n")
	_, _ = io.WriteString(w, "# TYPE bnpl_business_webhook_deliveries_total counter\n")
	webhookKeys := make([]string, 0, len(m.webhookDeliveries))
	for outcome := range m.webhookDeliveries {
		webhookKeys = append(webhookKeys, outcome)
	}

	sort.Strings(webhookKeys)
	for _, outcome := range webhookKeys {
		_, _ = fmt.Fprintf(
			w,
			"bnpl_business_webhook_deliveries_total{outcome=%s} %d\n",
			quoteLabel(outcome),
			m.webhookDeliveries[outcome],
		)
	}

	_, _ = io.WriteString(w, "# HELP bnpl_infrastructure_outbound_http_in_flight Current outbound HTTP requests holding an execution slot.\n")
	_, _ = io.WriteString(w, "# TYPE bnpl_infrastructure_outbound_http_in_flight gauge\n")
	_, _ = fmt.Fprintf(w, "bnpl_infrastructure_outbound_http_in_flight %d\n", m.outboundHTTPInFlight)

	_, _ = io.WriteString(w, "# HELP bnpl_infrastructure_outbound_http_queue_depth Current callers waiting in the bounded outbound HTTP queue.\n")
	_, _ = io.WriteString(w, "# TYPE bnpl_infrastructure_outbound_http_queue_depth gauge\n")
	_, _ = fmt.Fprintf(w, "bnpl_infrastructure_outbound_http_queue_depth %d\n", m.outboundHTTPQueueDepth)

	_, _ = io.WriteString(w, "# HELP bnpl_infrastructure_outbound_http_queue_wait_seconds Time spent waiting for an outbound HTTP execution slot.\n")
	_, _ = io.WriteString(w, "# TYPE bnpl_infrastructure_outbound_http_queue_wait_seconds summary\n")
	_, _ = fmt.Fprintf(w, "bnpl_infrastructure_outbound_http_queue_wait_seconds_sum %s\n", strconv.FormatFloat(m.outboundHTTPQueueWait.sum, 'g', -1, 64))
	_, _ = fmt.Fprintf(w, "bnpl_infrastructure_outbound_http_queue_wait_seconds_count %d\n", m.outboundHTTPQueueWait.count)

	_, _ = io.WriteString(w, "# HELP bnpl_infrastructure_outbound_http_saturated_total Total outbound HTTP requests rejected because both execution and queue capacity were full.\n")
	_, _ = io.WriteString(w, "# TYPE bnpl_infrastructure_outbound_http_saturated_total counter\n")
	_, _ = fmt.Fprintf(w, "bnpl_infrastructure_outbound_http_saturated_total %d\n", m.outboundHTTPSaturated)

	writeOutboundHTTPSeries(w, m.outboundHTTPRequests, m.outboundHTTPDuration)

}

// writeOutboundHTTPSeries emits outbound request counters and duration summaries
// in deterministic label order so scrape output is stable and testable.
func writeOutboundHTTPSeries(
	w io.Writer,
	requests map[outboundHTTPLabels]uint64,
	durations map[outboundHTTPLabels]durationSummary,
) {
	keys := make([]outboundHTTPLabels, 0, len(requests))
	for labels := range requests {
		keys = append(keys, labels)
	}

	sort.Slice(keys, func(i, j int) bool {
		if keys[i].method == keys[j].method {
			return keys[i].outcome < keys[j].outcome
		}

		return keys[i].method < keys[j].method
	})

	_, _ = io.WriteString(w, "# HELP bnpl_infrastructure_outbound_http_requests_total Total outbound HTTP attempts by bounded method and outcome.\n")
	_, _ = io.WriteString(w, "# TYPE bnpl_infrastructure_outbound_http_requests_total counter\n")
	for _, labels := range keys {
		_, _ = fmt.Fprintf(
			w,
			"bnpl_infrastructure_outbound_http_requests_total{method=%s,outcome=%s} %d\n",
			quoteLabel(labels.method),
			quoteLabel(labels.outcome),
			requests[labels],
		)
	}

	_, _ = io.WriteString(w, "# HELP bnpl_infrastructure_outbound_http_request_duration_seconds End-to-end client time to receive outbound HTTP response headers or an error, including bounded queue wait.\n")
	_, _ = io.WriteString(w, "# TYPE bnpl_infrastructure_outbound_http_request_duration_seconds summary\n")
	for _, labels := range keys {
		summary := durations[labels]
		_, _ = fmt.Fprintf(
			w,
			"bnpl_infrastructure_outbound_http_request_duration_seconds_sum{method=%s,outcome=%s} %s\n",
			quoteLabel(labels.method),
			quoteLabel(labels.outcome),
			strconv.FormatFloat(summary.sum, 'g', -1, 64),
		)
		_, _ = fmt.Fprintf(
			w,
			"bnpl_infrastructure_outbound_http_request_duration_seconds_count{method=%s,outcome=%s} %d\n",
			quoteLabel(labels.method),
			quoteLabel(labels.outcome),
			summary.count,
		)
	}
}

// writeSingleLabelCounter emits one counter family keyed by a single bounded
// label, sorting keys to keep exposition deterministic.
func writeSingleLabelCounter(w io.Writer, name, help, labelName string, values map[string]uint64) {
	_, _ = fmt.Fprintf(w, "# HELP %s %s\n", name, help)
	_, _ = fmt.Fprintf(w, "# TYPE %s counter\n", name)
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}

	sort.Strings(keys)
	for _, key := range keys {
		_, _ = fmt.Fprintf(w, "%s{%s=%s} %d\n", name, labelName, quoteLabel(key), values[key])
	}
}

// quoteLabel escapes and quotes a Prometheus label value safely.
func quoteLabel(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\n", "\\n")
	value = strings.ReplaceAll(value, "\"", "\\\"")
	return strconv.Quote(value)
}

// normalizeLabel keeps empty observations from creating an unlabeled series
// while still rejecting high-cardinality values at the call site by convention.
func normalizeLabel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}

	return value
}
