package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/parstis/bnpl-backend/internal/buildprofile"

	checkoutpaymentext "github.com/parstis/bnpl-backend/pkg/adapter/external/checkoutpayment"
	creditassessmentext "github.com/parstis/bnpl-backend/pkg/adapter/external/creditassessment"
	adaptercache "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/cache"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/circuitbreaker"
	adaptererrorreporting "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/errorreporting"
	adapterhttpclient "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/httpclient"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/messaging"
	adapterobjectstorage "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/objectstorage"
	taskadapter "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/task"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	coretask "github.com/parstis/bnpl-backend/pkg/core/task"
)

// Validate performs hierarchical post-load configuration validation.
func (c *Config) Validate() error {
	const op = "config.Config.Validate"

	if err := c.Runtime.validate(); err != nil {
		return err
	}

	if err := c.Database.validate(c.Runtime.Environment); err != nil {
		return err
	}

	if err := c.Gin.validate(c.Runtime.Environment); err != nil {
		return err
	}

	if err := c.Domain.validate(); err != nil {
		return err
	}

	if err := c.Usecases.validate(c.Runtime.Environment); err != nil {
		return err
	}

	if err := c.Infrastructure.validate(c.Runtime.Environment); err != nil {
		return err
	}

	// Contract cancellation uses durable Saga retries and compensation in every
	// environment. Running the application without the Saga job would make a
	// committed cancellation unable to recover from timeout or process failure.
	if !c.Infrastructure.Task.Scheduled.SagaExecutor.Enabled {
		return fmt.Errorf("%s: infrastructure.task.scheduled.saga_executor.enabled must be true while contract cancellation Saga is enabled", op)
	}

	if c.Infrastructure.Task.Provider == taskadapter.NoopProvider {
		return fmt.Errorf("%s: infrastructure.task.provider must not be noop while contract cancellation Saga is enabled", op)
	}

	env, err := model.ParseRuntimeEnvironment(c.Runtime.Environment)
	if err != nil {
		return fmt.Errorf("%s: invalid runtime.environment: %w", op, err)
	}

	// Issuance deliveries hold temporary API-key ciphertext. Production must
	// scrub expired rows into tombstones; correctness of replay refusal does not
	// depend on the worker, but leaving scrub disabled silently retains secrets.
	if isProductionMode(env) && !c.Infrastructure.Task.Scheduled.MerchantTerminalIssuanceScrub.Enabled {
		return fmt.Errorf(
			"%s: infrastructure.task.scheduled.merchant_terminal_issuance_scrub.enabled must be true in production",
			op,
		)
	}

	if c.Database.QueryLogging.Enabled && c.Infrastructure.Logger.Provider == log.NoopProvider {
		return fmt.Errorf("%s: database.query_logging.enabled requires a non-noop infrastructure logger", op)
	}

	if err := c.External.validate(c.Runtime.Environment, c.Infrastructure.HTTPClient); err != nil {
		return err
	}

	return nil
}

func (r Runtime) validate() error {
	const op = "config.Runtime.validate"

	if _, err := r.ParsedEnvironment(); err != nil {
		return fmt.Errorf("%s: invalid runtime.environment: %w", op, err)
	}

	return nil
}

func (d Database) validate(environment string) error {
	const op = "config.Database.validate"

	env, err := model.ParseRuntimeEnvironment(environment)
	if err != nil {
		return err
	}

	if isProductionMode(env) && d.QueryLogging.Enabled {
		return fmt.Errorf("%s: database.query_logging.enabled must be false in production", op)
	}

	if strings.TrimSpace(d.Host) == "" {
		return fmt.Errorf("%s: invalid database.host: value is required", op)
	}

	if d.Port <= 0 || d.Port > 65535 {
		return fmt.Errorf("%s: invalid database.port: must be between 1 and 65535", op)
	}

	if strings.TrimSpace(d.Name) == "" {
		return fmt.Errorf("%s: invalid database.name: value is required", op)
	}

	if d.PGPassEntries.Admin.IsEmpty() {
		return fmt.Errorf("%s: invalid database.pgpass_entries.admin: value is required", op)
	}

	if d.PGPassEntries.Normal.IsEmpty() {
		return fmt.Errorf("%s: invalid database.pgpass_entries.normal: value is required", op)
	}

	return nil
}

func (g Gin) validate(environment string) error {
	const op = "config.Gin.validate"

	env, err := model.ParseRuntimeEnvironment(environment)
	if err != nil {
		return err
	}

	// Production mode is true when either the compile-time build profile is
	// production or the configured runtime environment is production. Both
	// gates reject HTTPDebugLogger so a Development binary pointed at a
	// production environment cannot enable unredacted exchange logging.
	if isProductionMode(env) {
		if g.Swagger.Enabled {
			return fmt.Errorf("%s: gin.swagger.enabled must be false in production", op)
		}

		if g.HTTPDebugLogger.Enabled {
			return fmt.Errorf("%s: gin.http_debug_logger.enabled must be false in production", op)
		}
	}

	// Response logging enriches AccessLogger; enabling it without gin.logger
	// would silently do nothing. Fail closed with a clear config error instead.
	if g.ResponseLogging.Enabled && !g.Logger {
		return fmt.Errorf("%s: gin.response_logging.enabled requires gin.logger=true", op)
	}

	return nil
}

func (u *Usecases) validate(environment string) error {
	const op = "config.Usecases.validate"

	env, err := model.ParseRuntimeEnvironment(environment)
	if err != nil {
		return err
	}

	if strings.TrimSpace(u.Migrations.FilesDir) == "" {
		return fmt.Errorf("%s: invalid usecases.migration.files-dir: value is required", op)
	}

	if u.Migrations.LockTimeout < 0 {
		return fmt.Errorf("%s: invalid usecases.migration.lock_timeout: must not be negative", op)
	}

	if strings.TrimSpace(u.OutboxPublisher.WorkerID) == "" {
		return fmt.Errorf("%s: invalid usecases.outbox_publisher.worker_id: value is required", op)
	}

	if u.OutboxPublisher.BatchSize <= 0 {
		return fmt.Errorf("%s: invalid usecases.outbox_publisher.batch_size: must be positive", op)
	}

	if u.OutboxPublisher.RetryBaseDelay <= 0 {
		return fmt.Errorf("%s: invalid usecases.outbox_publisher.retry_base_delay: must be positive", op)
	}

	if u.OutboxPublisher.RetryMaxDelay < u.OutboxPublisher.RetryBaseDelay {
		return fmt.Errorf("%s: invalid usecases.outbox_publisher.retry_max_delay: must be greater than or equal to retry_base_delay", op)
	}

	if u.OutboxPublisher.ProcessingLeaseTimeout <= 0 {
		return fmt.Errorf("%s: invalid usecases.outbox_publisher.processing_lease_timeout: must be positive", op)
	}

	if _, _, err := u.CheckoutSession.resolvePublicOrigin(env); err != nil {
		return fmt.Errorf("%s: invalid usecases.checkout_session.public_origin: %w", op, err)
	}

	if u.CheckoutSession.TTL <= 0 {
		return fmt.Errorf("%s: invalid usecases.checkout_session.ttl: must be positive", op)
	}

	if u.CheckoutSession.ExpiryBatchSize <= 0 {
		return fmt.Errorf("%s: invalid usecases.checkout_session.expiry_batch_size: must be positive", op)
	}

	if strings.TrimSpace(u.CheckoutSession.LifecycleEventsDestination) == "" {
		return fmt.Errorf("%s: usecases.checkout_session.lifecycle_events_destination is required", op)
	}

	if err := u.CheckoutSession.validateSecrets(); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if err := u.CheckoutSession.validateOTP(); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if u.Installment.MarkDueBatchSize <= 0 {
		return fmt.Errorf("%s: invalid usecases.installment.mark_due_batch_size: must be positive", op)
	}

	if u.Installment.MarkOverdueBatchSize <= 0 {
		return fmt.Errorf("%s: invalid usecases.installment.mark_overdue_batch_size: must be positive", op)
	}

	if u.Contract.ExpiryBatchSize <= 0 {
		return fmt.Errorf("%s: invalid usecases.contract.expiry_batch_size: must be positive", op)
	}

	if u.Contract.PaymentResultProcessingGracePeriod < 0 {
		return fmt.Errorf("%s: invalid usecases.contract.payment_result_processing_grace_period: must not be negative", op)
	}

	u.SagaExecutor.ApplyDefaults()
	if err := u.SagaExecutor.validate(); err != nil {
		return err
	}

	u.Contract.CancellationSaga.ApplyDefaults()
	if err := u.Contract.CancellationSaga.validate(); err != nil {
		return err
	}

	if u.PaymentAttemptReconciliation.BatchSize <= 0 {
		return fmt.Errorf("%s: invalid usecases.payment_attempt_reconciliation.batch_size: must be positive", op)
	}

	if u.PaymentAttemptReconciliation.CreatedTimeout <= 0 {
		return fmt.Errorf("%s: invalid usecases.payment_attempt_reconciliation.created_timeout: must be positive", op)
	}

	if u.PaymentAttemptReconciliation.SubmittedExpiryGrace < 0 {
		return fmt.Errorf("%s: invalid usecases.payment_attempt_reconciliation.submitted_expiry_grace: must not be negative", op)
	}

	if u.PaymentAttemptReconciliation.RetryBaseDelay <= 0 {
		return fmt.Errorf("%s: invalid usecases.payment_attempt_reconciliation.retry_base_delay: must be positive", op)
	}

	if u.PaymentAttemptReconciliation.RetryMaxDelay < u.PaymentAttemptReconciliation.RetryBaseDelay {
		return fmt.Errorf("%s: invalid usecases.payment_attempt_reconciliation.retry_max_delay: must be greater than or equal to retry_base_delay", op)
	}

	if u.PaymentAttemptReconciliation.MaxAttempts < 0 {
		return fmt.Errorf("%s: invalid usecases.payment_attempt_reconciliation.max_attempts: must not be negative", op)
	}

	base, max := u.FinancialCorePaymentReconciliation.effectiveRetryDelays(u.PaymentAttemptReconciliation)
	if base <= 0 {
		return fmt.Errorf("%s: invalid usecases.financial_core_payment_reconciliation.retry_base_delay: effective value must be positive", op)
	}

	if max < base {
		return fmt.Errorf("%s: invalid usecases.financial_core_payment_reconciliation.retry_max_delay: effective value must be greater than or equal to retry_base_delay", op)
	}

	if err := u.MerchantTerminal.validate(); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (i Infrastructure) validate(environment string) error {
	const op = "config.Infrastructure.validate"

	env, err := model.ParseRuntimeEnvironment(environment)
	if err != nil {
		return err
	}

	if strings.TrimSpace(i.Clock.Location) == "" {
		return fmt.Errorf("%s: invalid infrastructure.clock.location: value is required", op)
	}

	if strings.TrimSpace(i.Clock.BusinessLocation) == "" {
		return fmt.Errorf("%s: invalid infrastructure.clock.business_location: value is required", op)
	}

	if err = i.Logger.validate(env); err != nil {
		return err
	}

	if err = i.ErrorReporting.validate(); err != nil {
		return err
	}

	if err = i.Metrics.Validate(); err != nil {
		return fmt.Errorf("%s: invalid infrastructure.metrics: %w", op, err)
	}

	if err = i.HTTPClient.validate(env); err != nil {
		return err
	}

	if err = i.Cache.validate(env); err != nil {
		return err
	}

	if err = i.ObjectStorage.validate(env); err != nil {
		return err
	}

	if err = i.Messaging.validate(env); err != nil {
		return err
	}

	if err = i.Task.validate(env); err != nil {
		return err
	}

	if err = i.CircuitBreaker.validate(env); err != nil {
		return err
	}

	return nil
}

func (l Logger) validate(env model.RuntimeEnvironment) error {
	const op = "config.Logger.validate"

	switch l.Provider {
	case log.SlogProvider, log.NoopProvider:

	default:
		return fmt.Errorf("%s: invalid infrastructure.logger.provider: unsupported value %q", op, l.Provider)
	}

	if isProductionMode(env) && l.Provider == log.NoopProvider {
		return fmt.Errorf("%s: infrastructure logger provider \"noop\" is not allowed in production", op)
	}

	return nil
}

func (e ErrorReporting) validate() error {
	const op = "config.ErrorReporting.validate"

	switch e.Provider {
	case "", adaptererrorreporting.NoopProvider:
		return nil

	case adaptererrorreporting.SentryProvider:
		if strings.TrimSpace(e.Sentry.DSN) == "" {
			return fmt.Errorf("%s: invalid infrastructure.error_reporting.sentry.dsn: value is required", op)
		}

		if e.Sentry.SampleRate <= 0 || e.Sentry.SampleRate > 1 {
			return fmt.Errorf("%s: invalid infrastructure.error_reporting.sentry.sample_rate: must be greater than 0 and less than or equal to 1", op)
		}

		return nil

	default:
		return fmt.Errorf("%s: invalid infrastructure.error_reporting.provider: unsupported value %q", op, e.Provider)
	}
}

func (h HTTPClient) validate(env model.RuntimeEnvironment) error {
	const op = "config.HTTPClient.validate"

	switch h.Provider {
	case adapterhttpclient.NoopProvider:
		if isProductionMode(env) {
			return fmt.Errorf("%s: http client provider \"noop\" is not allowed in production", op)
		}

		return nil

	case adapterhttpclient.StandardProvider:
		if h.Standard.MaxInFlight <= 0 {
			return fmt.Errorf("%s: invalid infrastructure.http_client.standard.max_in_flight: value must be positive", op)
		}

		if h.Standard.MaxQueueSize < 0 {
			return fmt.Errorf("%s: invalid infrastructure.http_client.standard.max_queue_size: value must be zero or positive", op)
		}

		if h.Standard.QueueWaitTimeout <= 0 {
			return fmt.Errorf("%s: invalid infrastructure.http_client.standard.queue_wait_timeout: value must be positive", op)
		}

		if h.Standard.Timeout <= 0 {
			return fmt.Errorf("%s: invalid infrastructure.http_client.standard.timeout: value must be positive", op)
		}

		if h.Standard.Transport.DialTimeout <= 0 {
			return fmt.Errorf("%s: invalid infrastructure.http_client.standard.transport.dial_timeout: value must be positive", op)
		}

		if h.Standard.Transport.TLSHandshakeTimeout <= 0 {
			return fmt.Errorf("%s: invalid infrastructure.http_client.standard.transport.tls_handshake_timeout: value must be positive", op)
		}

		if h.Standard.Transport.ResponseHeaderTimeout <= 0 {
			return fmt.Errorf("%s: invalid infrastructure.http_client.standard.transport.response_header_timeout: value must be positive", op)
		}

		if h.Standard.Transport.IdleConnTimeout <= 0 {
			return fmt.Errorf("%s: invalid infrastructure.http_client.standard.transport.idle_conn_timeout: value must be positive", op)
		}

		if h.Standard.Transport.MaxIdleConns <= 0 {
			return fmt.Errorf("%s: invalid infrastructure.http_client.standard.transport.max_idle_conns: value must be positive", op)
		}

		if h.Standard.Transport.MaxIdleConnsPerHost <= 0 {
			return fmt.Errorf("%s: invalid infrastructure.http_client.standard.transport.max_idle_conns_per_host: value must be positive", op)
		}

		if h.Standard.Transport.MaxConnsPerHost <= 0 {
			return fmt.Errorf("%s: invalid infrastructure.http_client.standard.transport.max_conns_per_host: value must be positive", op)
		}

		if h.Standard.Transport.MaxIdleConnsPerHost > h.Standard.Transport.MaxIdleConns {
			return fmt.Errorf("%s: invalid infrastructure.http_client.standard.transport.max_idle_conns_per_host: value must not exceed max_idle_conns", op)
		}

		return nil

	default:
		return fmt.Errorf("%s: invalid infrastructure.http_client.provider: unsupported value %q", op, h.Provider)
	}
}

func (c Cache) validate(env model.RuntimeEnvironment) error {
	const op = "config.Cache.validate"

	switch c.Provider {
	case adaptercache.NoopProvider, adaptercache.RedisProvider:

	default:
		return fmt.Errorf("%s: invalid infrastructure.cache.provider: unsupported value %q", op, c.Provider)
	}

	if c.Provider == adaptercache.RedisProvider && strings.TrimSpace(c.Redis.Addr) == "" {
		return fmt.Errorf("%s: invalid infrastructure.cache.redis.addr: value is required when cache provider is redis", op)
	}

	_ = env // cache noop is allowed in production (optimization only).

	return nil
}

func (o ObjectStorage) validate(env model.RuntimeEnvironment) error {
	const op = "config.ObjectStorage.validate"

	switch o.Provider {
	case "", adapterobjectstorage.NoopProvider:
		return nil

	case adapterobjectstorage.FilesystemProvider:
		if strings.TrimSpace(o.Filesystem.RootDir) == "" {
			return fmt.Errorf("%s: infrastructure.object_storage.filesystem.root_dir is required", op)
		}

		if isProductionMode(env) {
			return fmt.Errorf("%s: object storage provider filesystem is not allowed in production", op)
		}

		return nil

	default:
		return fmt.Errorf("%s: unsupported object storage provider %q", op, o.Provider)
	}
}

func (m Messaging) validate(env model.RuntimeEnvironment) error {
	const op = "config.Messaging.validate"

	switch m.Provider {
	case messaging.NoopProvider:
		if isProductionMode(env) {
			return fmt.Errorf("%s: messaging provider \"noop\" is not allowed in production", op)
		}

	case messaging.KafkaProvider:
		if len(m.Kafka.Brokers) == 0 || strings.TrimSpace(m.Kafka.Brokers[0]) == "" {
			return fmt.Errorf("%s: invalid infrastructure.messaging.kafka.brokers: at least one broker is required", op)
		}

		if strings.TrimSpace(m.Kafka.ClientID) == "" {
			return fmt.Errorf("%s: invalid infrastructure.messaging.kafka.client_id: value is required", op)
		}

		if strings.TrimSpace(m.Kafka.GroupID) == "" {
			return fmt.Errorf("%s: invalid infrastructure.messaging.kafka.group_id: value is required", op)
		}

		if err := m.Kafka.Topics.validate(); err != nil {
			return err
		}

		if err := m.Kafka.Consumer.validateInitialOffset(env); err != nil {
			return err
		}

		quarantineTopic := m.Kafka.Consumer.quarantineTopic(m.Kafka.GroupID)
		if quarantineTopic == "" {
			return fmt.Errorf("%s: invalid infrastructure.messaging.kafka.consumer.quarantine_topic: value or group_id is required", op)
		}

		for _, topic := range m.Kafka.Consumer.Topics {
			if strings.TrimSpace(topic) == quarantineTopic {
				return fmt.Errorf("%s: infrastructure.messaging.kafka.consumer.quarantine_topic must not be consumed by the same consumer group", op)
			}
		}

	default:
		return fmt.Errorf("%s: invalid infrastructure.messaging.provider: unsupported value %q", op, m.Provider)
	}

	return nil
}

func (c Consumer) validateInitialOffset(env model.RuntimeEnvironment) error {
	const op = "config.Consumer.validateInitialOffset"

	policy, err := messaging.ParseInitialOffset(c.InitialOffset)
	if err != nil {
		return fmt.Errorf("%s: invalid infrastructure.messaging.kafka.consumer.initial_offset: %w", op, err)
	}

	if isProductionMode(env) && policy == messaging.InitialOffsetNewest {
		return fmt.Errorf(
			"%s: infrastructure.messaging.kafka.consumer.initial_offset newest is not allowed in production because backlog would be skipped", op,
		)
	}

	if policy == messaging.InitialOffsetNewest && !c.AllowNewestInitialOffset {
		return fmt.Errorf(
			"%s: infrastructure.messaging.kafka.consumer.initial_offset newest requires allow_newest_initial_offset=true outside production", op,
		)
	}

	return nil
}

func (t Topics) validate() error {
	const op = "config.Topics.validate"

	required := map[string]string{
		"infrastructure.messaging.kafka.topics.payment_results":                                     t.PaymentResults,
		"infrastructure.messaging.kafka.topics.audit_events":                                        t.AuditEvents,
		"infrastructure.messaging.kafka.topics.payment_application_results":                         t.PaymentApplicationResults,
		"infrastructure.messaging.kafka.topics.installment_payment_requests":                        t.InstallmentPaymentRequests,
		"infrastructure.messaging.kafka.topics.installment_payment_sessions":                        t.InstallmentPaymentSessions,
		"infrastructure.messaging.kafka.topics.installment_payment_reconciliation_requests":         t.InstallmentPaymentReconciliationRequests,
		"infrastructure.messaging.kafka.topics.installment_payment_reconciliation_results":          t.InstallmentPaymentReconciliationResults,
		"infrastructure.messaging.kafka.topics.installment_payment_reconciliation_acknowledgements": t.InstallmentPaymentReconciliationAcknowledgements,
	}

	for field, value := range required {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s: invalid %s: value is required when messaging provider is kafka", op, field)
		}
	}

	return nil
}

func (t Task) validate(env model.RuntimeEnvironment) error {
	const op = "config.Task.validate"

	switch t.Provider {
	case taskadapter.NoopProvider:
		// Background jobs are part of the production correctness model. A noop
		// runtime would silently disable expiry, reconciliation, Saga recovery,
		// and other scheduled work, so production must fail closed.
		if isProductionMode(env) {
			return fmt.Errorf("%s: task provider \"noop\" is not allowed in production", op)
		}

		return t.validateStandaloneTaskExecutions()

	case taskadapter.InProcessProvider:
		if err := t.validateInProcessTaskRuntime(); err != nil {
			return err
		}

		return t.validateStandaloneTaskExecutions()

	case taskadapter.TemporalProvider:
		if err := t.validateTemporalTaskRuntime(); err != nil {
			return err
		}

		return t.validateStandaloneTaskExecutions()

	case taskadapter.CompositeProvider:
		if err := t.validateInProcessTaskRuntime(); err != nil {
			return err
		}

		if err := t.validateTemporalTaskRuntime(); err != nil {
			return err
		}

		return t.validateCompositeTaskExecutions()

	default:
		return fmt.Errorf("%s: invalid infrastructure.task.provider: unsupported value %q", op, t.Provider)
	}
}

func (t Task) validateInProcessTaskRuntime() error {
	const op = "config.Task.validateInProcessTaskRuntime"

	if strings.TrimSpace(t.InProcess.DefaultTimezone) == "" {
		return fmt.Errorf("%s: invalid infrastructure.task.inprocess.default_timezone: value is required", op)
	}

	if t.InProcess.MaxConcurrentRuns < 0 {
		return fmt.Errorf("%s: invalid infrastructure.task.inprocess.max_concurrent_runs: must not be negative", op)
	}

	return nil
}

func (t Task) validateTemporalTaskRuntime() error {
	const op = "config.Task.validateTemporalTaskRuntime"

	if strings.TrimSpace(t.Temporal.Address) == "" {
		return fmt.Errorf("%s: invalid infrastructure.task.temporal.address: value is required", op)
	}

	if strings.TrimSpace(t.Temporal.Namespace) == "" {
		return fmt.Errorf("%s: invalid infrastructure.task.temporal.namespace: value is required", op)
	}

	if strings.TrimSpace(t.Temporal.TaskQueue) == "" {
		return fmt.Errorf("%s: invalid infrastructure.task.temporal.task_queue: value is required", op)
	}

	if t.Temporal.ActivityStartToCloseTimeout <= 0 {
		return fmt.Errorf("%s: invalid infrastructure.task.temporal.activity_start_to_close_timeout: must be positive", op)
	}

	if t.Temporal.Worker.MaxConcurrentActivities < 0 {
		return fmt.Errorf("%s: invalid infrastructure.task.temporal.worker.max_concurrent_activities: must not be negative", op)
	}

	if t.Temporal.Worker.MaxConcurrentWorkflowTasks < 0 {
		return fmt.Errorf("%s: invalid infrastructure.task.temporal.worker.max_concurrent_workflow_tasks: must not be negative", op)
	}

	// Temporal SDK rejects a workflow-task concurrency of one because at least
	// one additional slot is required for sticky-queue progress. Zero delegates
	// to the SDK default.
	if t.Temporal.Worker.MaxConcurrentWorkflowTasks == 1 {
		return fmt.Errorf("%s: invalid infrastructure.task.temporal.worker.max_concurrent_workflow_tasks: must be zero or greater than one", op)
	}

	return nil
}

func (t Task) validateStandaloneTaskExecutions() error {
	const op = "config.Task.validateStandaloneTaskExecutions"

	definitions, err := t.getTaskDefinitions()
	if err != nil {
		return fmt.Errorf("%s: get task definitions: %w", op, err)
	}

	for _, definition := range definitions {
		if definition.Execution != "" {
			return fmt.Errorf(
				"%s: invalid infrastructure.task.scheduled.%s.execution: must be empty when provider=%s", op, definition.Name,
				t.Provider,
			)
		}
	}

	return nil
}

func (t Task) validateCompositeTaskExecutions() error {
	const op = "config.Task.validateCompositeTaskExecutions"

	definitions, err := t.getTaskDefinitions()
	if err != nil {
		return fmt.Errorf("%s: get task definitions: %w", op, err)
	}

	for _, definition := range definitions {
		// Disabled definitions may omit execution. If a value is present, still
		// validate it so enabling the task later cannot reveal a latent typo.
		if !definition.Enabled && definition.Execution == "" {
			continue
		}

		switch definition.Execution {
		case coretask.ExecutionModeInProcess, coretask.ExecutionModeTemporal:
			continue

		case "":
			return fmt.Errorf(
				"%s: invalid infrastructure.task.scheduled.%s.execution: value is required when provider=composite", op, definition.Name,
			)

		default:
			return fmt.Errorf(
				"%s: invalid infrastructure.task.scheduled.%s.execution: unsupported value %q", op, definition.Name,
				definition.Execution,
			)
		}
	}

	return nil
}

func (c CircuitBreaker) validate(env model.RuntimeEnvironment) error {
	const op = "config.CircuitBreaker.validate"

	switch c.Provider {
	case circuitbreaker.NoopProvider:
		if isProductionMode(env) {
			return fmt.Errorf("%s: circuit breaker provider \"noop\" is not allowed in production", op)
		}

	case circuitbreaker.GoBreakerProvider:

	default:
		return fmt.Errorf("%s: invalid infrastructure.circuit_breaker.provider: unsupported value %q", op, c.Provider)
	}

	return nil
}

func (e External) validate(environment string, httpClient HTTPClient) error {
	env, err := model.ParseRuntimeEnvironment(environment)
	if err != nil {
		return err
	}

	if err := e.CreditAssessment.validate(env); err != nil {
		return err
	}

	if err := e.FinancialCore.validate(env); err != nil {
		return err
	}

	if err := e.Identity.validate(env); err != nil {
		return err
	}

	if err := e.Notification.validate(env); err != nil {
		return err
	}

	if err := e.BotMessaging.validate(httpClient); err != nil {
		return err
	}

	e.WebhookDispatcher.ApplyDefaults()
	if err := e.WebhookDispatcher.validate(env); err != nil {
		return err
	}

	return nil
}

func (e ExternalBotMessaging) validate(httpClient HTTPClient) error {
	const op = "config.ExternalBotMessaging.validate"

	realProviders := 0
	if e.Telegram.Enabled {
		realProviders++
	}

	if e.Bale.Enabled {
		realProviders++
	}

	if e.Noop.Enabled && realProviders > 0 {
		return fmt.Errorf("%s: external.bot_messaging.noop.enabled must be false when telegram or bale is enabled", op)
	}

	// All-disabled is treated as implicit noop for backward compatibility with
	// pre-bot-messaging configuration files. The canonical shipped config sets
	// noop.enabled explicitly.
	if !e.Noop.Enabled && realProviders == 0 {
		return nil
	}

	if e.Noop.Enabled {
		return nil
	}

	if httpClient.Provider != adapterhttpclient.StandardProvider {
		return fmt.Errorf("%s: external.bot_messaging real providers require infrastructure.http_client.provider=standard", op)
	}

	validateProvider := func(name string, token string, pollTimeout, retryBaseDelay, retryMaxDelay time.Duration) error {
		prefix := "external.bot_messaging." + name
		if strings.TrimSpace(token) == "" {
			return fmt.Errorf("%s: invalid %s.token: value is required", op, prefix)
		}

		if pollTimeout < time.Second {
			return fmt.Errorf("%s: invalid %s.poll_timeout: must be at least 1s", op, prefix)
		}

		if retryBaseDelay <= 0 {
			return fmt.Errorf("%s: invalid %s.retry_base_delay: must be positive", op, prefix)
		}

		if retryMaxDelay < retryBaseDelay {
			return fmt.Errorf("%s: invalid %s.retry_max_delay: must be >= retry_base_delay", op, prefix)
		}

		// Long polling delays response headers by design. The shared outbound
		// client must therefore allow more time than the provider-side poll.
		if httpClient.Standard.Transport.ResponseHeaderTimeout <= pollTimeout {
			return fmt.Errorf(
				"%s: invalid %s.poll_timeout: must be less than infrastructure.http_client.standard.transport.response_header_timeout", op, prefix,
			)
		}

		if httpClient.Standard.Timeout <= pollTimeout {
			return fmt.Errorf(
				"%s: invalid %s.poll_timeout: must be less than infrastructure.http_client.standard.timeout", op, prefix,
			)
		}

		return nil
	}

	if e.Telegram.Enabled {
		if err := validateProvider(
			"telegram",
			e.Telegram.Token.Reveal(),
			e.Telegram.PollTimeout,
			e.Telegram.RetryBaseDelay,
			e.Telegram.RetryMaxDelay,
		); err != nil {
			return err
		}
	}

	if e.Bale.Enabled {
		if err := validateProvider(
			"bale",
			e.Bale.Token.Reveal(),
			e.Bale.PollTimeout,
			e.Bale.RetryBaseDelay,
			e.Bale.RetryMaxDelay,
		); err != nil {
			return err
		}
	}

	return nil
}

func (e ExternalCreditAssessment) validate(env model.RuntimeEnvironment) error {
	const op = "config.ExternalCreditAssessment.validate"

	provider := creditassessmentext.Provider(e.Provider)

	switch provider {
	case creditassessmentext.NoopProvider:
		if isProductionMode(env) {
			return fmt.Errorf("%s: external credit assessment provider \"noop\" is not allowed in production", op)
		}

	case creditassessmentext.CreditBureauProvider:

	default:
		return fmt.Errorf("%s: invalid external.credit_assessment.provider: unsupported value %q", op, e.Provider)
	}

	return nil
}

func (e ExternalFinancialCore) validate(env model.RuntimeEnvironment) error {
	const op = "config.ExternalFinancialCore.validate"

	provider := checkoutpaymentext.Provider(e.Provider)

	switch provider {
	case checkoutpaymentext.NoopProvider:
		if isProductionMode(env) {
			return fmt.Errorf("%s: external financial core provider \"noop\" is not allowed in production", op)
		}

	case checkoutpaymentext.GRPCProvider:
		if strings.TrimSpace(e.GRPC.Address) == "" {
			return fmt.Errorf("%s: invalid external.financial_core.grpc.address: value is required", op)
		}

		if e.GRPC.Timeout <= 0 {
			return fmt.Errorf("%s: invalid external.financial_core.grpc.timeout: must be positive", op)
		}

		if isProductionMode(env) && e.GRPC.Insecure {
			return fmt.Errorf("%s: external.financial_core.grpc.insecure must be false in production", op)
		}

	default:
		return fmt.Errorf("%s: invalid external.financial_core.provider: unsupported value %q", op, e.Provider)
	}

	if err := validateHTTPSURL("external.financial_core.payment_return_url", e.PaymentReturnURL); err != nil {
		return err
	}

	return nil
}

// isProductionMode applies production safety constraints whenever either the
// immutable binary profile or the runtime deployment environment is production.
// Runtime configuration can tighten a development build, but it can never
// relax a production build.
func isProductionMode(env model.RuntimeEnvironment) bool {
	return buildprofile.IsProduction() || env.IsProduction()
}
