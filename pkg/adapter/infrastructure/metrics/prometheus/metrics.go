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

	shortLinksCreated       map[string]uint64
	shortLinksResolved      map[string]uint64
	shortLinkCollisions     uint64
	shortLinkResolveLatency durationSummary

	repaymentReminderJobs              map[string]uint64
	repaymentReminderJobDuration       durationSummary
	repaymentReminderCandidates        uint64
	repaymentReminderEligibleCustomers uint64
	repaymentRemindersCreated          map[string]uint64
	repaymentRemindersSkipped          map[string]uint64

	collectionsAssessmentJobs        map[string]uint64
	collectionsAssessmentJobDuration durationSummary
	collectionsAssessmentCustomers   uint64
	collectionsDefaulted             uint64
	collectionsRecovered             uint64
	collectionsAssessmentSkipped     uint64

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
		checkoutDecisions:         make(map[string]uint64),
		contracts:                 make(map[string]uint64),
		paymentAttempts:           make(map[paymentAttemptLabels]uint64),
		installmentStates:         make(map[installmentStateLabels]uint64),
		webhookDeliveries:         make(map[string]uint64),
		shortLinksCreated:         make(map[string]uint64),
		shortLinksResolved:        make(map[string]uint64),
		repaymentReminderJobs:     make(map[string]uint64),
		repaymentRemindersCreated: make(map[string]uint64),
		repaymentRemindersSkipped: make(map[string]uint64),
		collectionsAssessmentJobs: make(map[string]uint64),
		outboundHTTPRequests:      make(map[outboundHTTPLabels]uint64),
		outboundHTTPDuration:      make(map[outboundHTTPLabels]durationSummary),
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

// ShortLinkCreated increments bnpl_business_short_links_created_total.
func (m *Metrics) ShortLinkCreated(_ context.Context, mode string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.shortLinksCreated[normalizeLabel(mode)]++
}

// ShortLinkResolved increments bnpl_business_short_links_resolved_total.
func (m *Metrics) ShortLinkResolved(_ context.Context, outcome string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.shortLinksResolved[normalizeLabel(outcome)]++
}

// ShortLinkCodeCollision increments bnpl_business_short_link_code_collisions_total.
func (m *Metrics) ShortLinkCodeCollision(_ context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.shortLinkCollisions++
}

// ShortLinkResolveLatency records bnpl_business_short_link_resolve_latency_seconds.
func (m *Metrics) ShortLinkResolveLatency(_ context.Context, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.shortLinkResolveLatency.count++
	m.shortLinkResolveLatency.sum += duration.Seconds()
}

// RepaymentReminderJobCompleted increments bnpl_business_repayment_reminder_jobs_total.
func (m *Metrics) RepaymentReminderJobCompleted(_ context.Context, outcome string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.repaymentReminderJobs[normalizeLabel(outcome)]++
}

// RepaymentReminderJobDuration records bnpl_business_repayment_reminder_job_duration_seconds.
func (m *Metrics) RepaymentReminderJobDuration(_ context.Context, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.repaymentReminderJobDuration.count++
	m.repaymentReminderJobDuration.sum += duration.Seconds()
}

// RepaymentReminderCandidatesObserved adds to bnpl_business_repayment_reminder_candidates_total.
func (m *Metrics) RepaymentReminderCandidatesObserved(_ context.Context, count int) {
	if count <= 0 {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.repaymentReminderCandidates += uint64(count)
}

// RepaymentReminderEligibleCustomersObserved adds to
// bnpl_business_repayment_reminder_eligible_customers_total.
func (m *Metrics) RepaymentReminderEligibleCustomersObserved(_ context.Context, count int) {
	if count <= 0 {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.repaymentReminderEligibleCustomers += uint64(count)
}

// RepaymentReminderCreated increments bnpl_business_repayment_reminders_created_total.
func (m *Metrics) RepaymentReminderCreated(_ context.Context, reminderType string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.repaymentRemindersCreated[normalizeLabel(reminderType)]++
}

// RepaymentReminderSkipped increments bnpl_business_repayment_reminders_skipped_total.
func (m *Metrics) RepaymentReminderSkipped(_ context.Context, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.repaymentRemindersSkipped[normalizeLabel(reason)]++
}

// CollectionsAssessmentJobCompleted increments bnpl_business_collections_assessment_jobs_total.
func (m *Metrics) CollectionsAssessmentJobCompleted(_ context.Context, outcome string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.collectionsAssessmentJobs[normalizeLabel(outcome)]++
}

// CollectionsAssessmentJobDuration records bnpl_business_collections_assessment_job_duration_seconds.
func (m *Metrics) CollectionsAssessmentJobDuration(_ context.Context, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.collectionsAssessmentJobDuration.count++
	m.collectionsAssessmentJobDuration.sum += duration.Seconds()
}

// CollectionsAssessmentCustomersObserved adds to bnpl_business_collections_assessment_customers_total.
func (m *Metrics) CollectionsAssessmentCustomersObserved(_ context.Context, count int) {
	if count <= 0 {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.collectionsAssessmentCustomers += uint64(count)
}

// CollectionsDefaulted adds to bnpl_business_collections_defaulted_total.
func (m *Metrics) CollectionsDefaulted(_ context.Context, count int) {
	if count <= 0 {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.collectionsDefaulted += uint64(count)
}

// CollectionsRecovered adds to bnpl_business_collections_recovered_total.
func (m *Metrics) CollectionsRecovered(_ context.Context, count int) {
	if count <= 0 {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.collectionsRecovered += uint64(count)
}

// CollectionsAssessmentSkipped adds to bnpl_business_collections_assessment_skipped_total.
func (m *Metrics) CollectionsAssessmentSkipped(_ context.Context, count int) {
	if count <= 0 {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.collectionsAssessmentSkipped += uint64(count)
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

	writeSingleLabelCounter(
		w,
		"bnpl_business_short_links_created_total",
		"Total short links persisted by creation mode.",
		"mode",
		m.shortLinksCreated,
	)
	writeSingleLabelCounter(
		w,
		"bnpl_business_short_links_resolved_total",
		"Total short-link resolutions by outcome.",
		"outcome",
		m.shortLinksResolved,
	)

	_, _ = io.WriteString(w, "# HELP bnpl_business_short_link_code_collisions_total Total short-link code collisions during creation.\n")
	_, _ = io.WriteString(w, "# TYPE bnpl_business_short_link_code_collisions_total counter\n")
	_, _ = fmt.Fprintf(w, "bnpl_business_short_link_code_collisions_total %d\n", m.shortLinkCollisions)

	_, _ = io.WriteString(w, "# HELP bnpl_business_short_link_resolve_latency_seconds Short-link resolution latency.\n")
	_, _ = io.WriteString(w, "# TYPE bnpl_business_short_link_resolve_latency_seconds summary\n")
	_, _ = fmt.Fprintf(
		w,
		"bnpl_business_short_link_resolve_latency_seconds_sum %s\n",
		strconv.FormatFloat(m.shortLinkResolveLatency.sum, 'g', -1, 64),
	)
	_, _ = fmt.Fprintf(
		w,
		"bnpl_business_short_link_resolve_latency_seconds_count %d\n",
		m.shortLinkResolveLatency.count,
	)

	writeSingleLabelCounter(
		w,
		"bnpl_business_repayment_reminder_jobs_total",
		"Total repayment reminder dispatch job executions by outcome.",
		"outcome",
		m.repaymentReminderJobs,
	)

	_, _ = io.WriteString(w, "# HELP bnpl_business_repayment_reminder_job_duration_seconds Repayment reminder dispatch job duration.\n")
	_, _ = io.WriteString(w, "# TYPE bnpl_business_repayment_reminder_job_duration_seconds summary\n")
	_, _ = fmt.Fprintf(
		w,
		"bnpl_business_repayment_reminder_job_duration_seconds_sum %s\n",
		strconv.FormatFloat(m.repaymentReminderJobDuration.sum, 'g', -1, 64),
	)
	_, _ = fmt.Fprintf(
		w,
		"bnpl_business_repayment_reminder_job_duration_seconds_count %d\n",
		m.repaymentReminderJobDuration.count,
	)

	_, _ = io.WriteString(w, "# HELP bnpl_business_repayment_reminder_candidates_total Total candidate installments observed by reminder dispatch.\n")
	_, _ = io.WriteString(w, "# TYPE bnpl_business_repayment_reminder_candidates_total counter\n")
	_, _ = fmt.Fprintf(w, "bnpl_business_repayment_reminder_candidates_total %d\n", m.repaymentReminderCandidates)

	_, _ = io.WriteString(w, "# HELP bnpl_business_repayment_reminder_eligible_customers_total Total eligible customers observed by reminder dispatch.\n")
	_, _ = io.WriteString(w, "# TYPE bnpl_business_repayment_reminder_eligible_customers_total counter\n")
	_, _ = fmt.Fprintf(
		w,
		"bnpl_business_repayment_reminder_eligible_customers_total %d\n",
		m.repaymentReminderEligibleCustomers,
	)

	writeSingleLabelCounter(
		w,
		"bnpl_business_repayment_reminders_created_total",
		"Total durably persisted repayment reminder requests by type.",
		"reminder_type",
		m.repaymentRemindersCreated,
	)
	writeSingleLabelCounter(
		w,
		"bnpl_business_repayment_reminders_skipped_total",
		"Total skipped repayment reminders by bounded reason.",
		"reason",
		m.repaymentRemindersSkipped,
	)

	writeSingleLabelCounter(
		w,
		"bnpl_business_collections_assessment_jobs_total",
		"Total collections assessment job executions by outcome.",
		"outcome",
		m.collectionsAssessmentJobs,
	)

	_, _ = io.WriteString(w, "# HELP bnpl_business_collections_assessment_job_duration_seconds Collections assessment job duration.\n")
	_, _ = io.WriteString(w, "# TYPE bnpl_business_collections_assessment_job_duration_seconds summary\n")
	_, _ = fmt.Fprintf(
		w,
		"bnpl_business_collections_assessment_job_duration_seconds_sum %s\n",
		strconv.FormatFloat(m.collectionsAssessmentJobDuration.sum, 'g', -1, 64),
	)
	_, _ = fmt.Fprintf(
		w,
		"bnpl_business_collections_assessment_job_duration_seconds_count %d\n",
		m.collectionsAssessmentJobDuration.count,
	)

	_, _ = io.WriteString(w, "# HELP bnpl_business_collections_assessment_customers_total Total customers assessed by collections.\n")
	_, _ = io.WriteString(w, "# TYPE bnpl_business_collections_assessment_customers_total counter\n")
	_, _ = fmt.Fprintf(w, "bnpl_business_collections_assessment_customers_total %d\n", m.collectionsAssessmentCustomers)

	_, _ = io.WriteString(w, "# HELP bnpl_business_collections_defaulted_total Total newly opened default episodes.\n")
	_, _ = io.WriteString(w, "# TYPE bnpl_business_collections_defaulted_total counter\n")
	_, _ = fmt.Fprintf(w, "bnpl_business_collections_defaulted_total %d\n", m.collectionsDefaulted)

	_, _ = io.WriteString(w, "# HELP bnpl_business_collections_recovered_total Total recovered default episodes.\n")
	_, _ = io.WriteString(w, "# TYPE bnpl_business_collections_recovered_total counter\n")
	_, _ = fmt.Fprintf(w, "bnpl_business_collections_recovered_total %d\n", m.collectionsRecovered)

	_, _ = io.WriteString(w, "# HELP bnpl_business_collections_assessment_skipped_total Total assessments with no state transition.\n")
	_, _ = io.WriteString(w, "# TYPE bnpl_business_collections_assessment_skipped_total counter\n")
	_, _ = fmt.Fprintf(w, "bnpl_business_collections_assessment_skipped_total %d\n", m.collectionsAssessmentSkipped)

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
func writeSingleLabelCounter(
	w io.Writer,
	name, help, labelName string,
	values map[string]uint64,
) {
	_, _ = fmt.Fprintf(w, "# HELP %s %s\n", name, help)
	_, _ = fmt.Fprintf(w, "# TYPE %s counter\n", name)
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}

	sort.Strings(keys)
	for _, key := range keys {
		_, _ = fmt.Fprintf(
			w,
			"%s{%s=%s} %d\n",
			name,
			labelName,
			quoteLabel(key),
			values[key],
		)
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
