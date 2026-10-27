package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	checkoutpaymentext "github.com/parstis/bnpl-backend/pkg/adapter/external/checkoutpayment"
	creditassessmentext "github.com/parstis/bnpl-backend/pkg/adapter/external/creditassessment"
	customerext "github.com/parstis/bnpl-backend/pkg/adapter/external/customer"
	"github.com/parstis/bnpl-backend/pkg/adapter/external/merchantwebhook"
	notificationext "github.com/parstis/bnpl-backend/pkg/adapter/external/notification"
	adaptercache "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/cache"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/circuitbreaker"
	adaptererrorreporting "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/errorreporting"
	adapterhttpclient "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/httpclient"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/messaging"
	taskadapter "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/task"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	coretask "github.com/parstis/bnpl-backend/pkg/core/task"
)

// requireProductionIdentityAndNotification configures non-noop Identity and
// Notification gRPC settings required for production validation fixtures.
func requireProductionIdentityAndNotification(cfg *Config) {
	cfg.External.Customer.Provider = string(customerext.GRPCProvider)
	cfg.External.Customer.GRPC.Address = "127.0.0.1:50053"
	cfg.External.Customer.GRPC.Timeout = time.Second
	cfg.External.Notification.Provider = string(notificationext.GRPCProvider)
	cfg.External.Notification.GRPC.Address = "127.0.0.1:50054"
	cfg.External.Notification.GRPC.Timeout = time.Second
}

func enabledSagaExecutorTask(provider taskadapter.Provider) Task {
	return Task{
		Provider: provider,
		InProcess: InProcess{
			DefaultTimezone: "UTC",
		},
		Scheduled: ScheduledTasks{
			SagaExecutor: TaskDefinition{
				Enabled: true,
				Handler: "saga_executor",
				Trigger: TaskTrigger{
					Type:     "interval",
					Interval: "10s",
				},
			},
			MerchantTerminalIssuanceScrub: TaskDefinition{
				Enabled: true,
				Handler: "merchant_terminal_issuance_scrub",
				Trigger: TaskTrigger{
					Type:     "interval",
					Interval: "5m",
				},
			},
		},
	}
}

func validHTTPClient() HTTPClient {
	return HTTPClient{
		Provider: adapterhttpclient.StandardProvider,
		Standard: HTTPClientStandard{
			MaxInFlight:      50,
			MaxQueueSize:     100,
			QueueWaitTimeout: 500 * time.Millisecond,
			Timeout:          5 * time.Second,
			Transport: HTTPClientTransport{
				DialTimeout:           2 * time.Second,
				TLSHandshakeTimeout:   2 * time.Second,
				ResponseHeaderTimeout: 3 * time.Second,
				IdleConnTimeout:       90 * time.Second,
				MaxIdleConns:          100,
				MaxIdleConnsPerHost:   20,
				MaxConnsPerHost:       50,
			},
		},
	}
}

func validConfig() *Config {
	return &Config{
		Runtime: Runtime{Environment: "development"},
		Domain: Domain{
			SupportedJurisdictions: []model.Jurisdiction{model.IRJurisdiction},
		},
		Database: Database{
			Host: "localhost",
			Port: 5432,
			Name: "bnpl",
			PGPassEntries: struct {
				Admin  model.Secret `yaml:"admin"`
				Normal model.Secret `yaml:"normal"`
			}{
				Admin:  model.NewSecret("localhost:5432:bnpl:admin:secret"),
				Normal: model.NewSecret("localhost:5432:bnpl:bnpl:secret"),
			},
		},
		Usecases: Usecases{
			CheckoutSession: CheckoutSession{
				PublicOrigin: "https://bnpl.example",
				PublicOriginPolicy: PublicOriginPolicyConfig{
					AllowedHosts: []string{"bnpl.example"},
				},
				TTL:                        30 * time.Minute,
				ExpiryBatchSize:            100,
				LifecycleEventsDestination: "bnpl.merchant.checkout.events.v1",
				WebhookSecretEncryptionKeys: KeyEntries{
					{ID: "w1", Secret: model.NewSecret("ZGV2LW9ubHktd2ViaG9vay1lbmNyeXB0aW9uLWtleSE=")},
				},
				CustomerPhoneEncryptionKeys: KeyEntries{
					{ID: "p1", Secret: model.NewSecret("ZGV2LW9ubHktY3VzdG9tZXItcGhvbmUta2V5LTAxMjM=")},
				},
				OTPPepperKeys: KeyEntries{
					{ID: "o1", Secret: model.NewSecret("test-checkout-otp-pepper-0123456789ab")},
				},
			},
			Migrations: Migrations{FilesDir: "assets/db/migrations"},
			OutboxPublisher: OutboxPublisher{
				WorkerID:               "worker-1",
				BatchSize:              10,
				RetryBaseDelay:         time.Second,
				RetryMaxDelay:          time.Minute,
				ProcessingLeaseTimeout: time.Minute,
			},
			Installment: Installment{
				MarkDueBatchSize:     100,
				MarkOverdueBatchSize: 100,
			},
			Contract: Contract{
				PaymentResultProcessingGracePeriod: time.Minute,
				ExpiryBatchSize:                    100,
			},
			PaymentAttemptReconciliation: PaymentAttemptReconciliation{
				BatchSize:      100,
				CreatedTimeout: time.Minute,
				RetryBaseDelay: time.Second,
				RetryMaxDelay:  time.Minute,
			},
			MerchantTerminal: MerchantTerminal{
				APIKeyPepper: model.NewSecret("test-merchant-terminal-api-key-pepper-32b"),
				Rotation: MerchantTerminalRotation{
					DefaultGracePeriod: 24 * time.Hour,
					MaxGracePeriod:     7 * 24 * time.Hour,
				},
				IssuanceDeliveryTTL: 24 * time.Hour,
				IssuanceEncryptionKeys: KeyEntries{
					{ID: "m1", Secret: model.NewSecret("dGVzdC1tZXJjaGFudC1hdXRoLWlzc3VhbmNlLWtleSE=")},
				},
			},
		},
		Infrastructure: Infrastructure{
			Clock: Clock{
				Location:         "UTC",
				BusinessLocation: "UTC",
			},
			Logger:         Logger{Provider: log.NoopProvider},
			HTTPClient:     validHTTPClient(),
			Cache:          Cache{Provider: adaptercache.NoopProvider},
			Messaging:      Messaging{Provider: messaging.NoopProvider},
			Task:           enabledSagaExecutorTask(taskadapter.InProcessProvider),
			CircuitBreaker: CircuitBreaker{Provider: circuitbreaker.NoopProvider},
		},
		External: External{
			CreditAssessment: ExternalCreditAssessment{
				Provider: string(creditassessmentext.NoopProvider),
			},
			FinancialCore: ExternalFinancialCore{
				Provider:         string(checkoutpaymentext.NoopProvider),
				PaymentReturnURL: "https://bnpl.example/payments/return",
			},
			Customer: ExternalCustomer{
				Provider: string(customerext.NoopProvider),
			},
			Notification: ExternalNotification{
				Provider: string(notificationext.NoopProvider),
			},
			WebhookDispatcher: ExternalWebhookDispatcher{
				Provider: string(merchantwebhook.HTTPProvider),
				HTTP: ExternalWebhookDispatcherHTTP{
					Timeout:          10 * time.Second,
					DialTimeout:      5 * time.Second,
					MaxResponseBytes: 65536,
				},
			},
		},
		Versions: Versions{
			Config:   Version,
			Database: DatabaseVersion,
		},
	}
}

func TestErrorReportingValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  ErrorReporting
		wantErr string
	}{
		{
			name:   "empty provider defaults to noop",
			config: ErrorReporting{},
		},
		{
			name:   "explicit noop",
			config: ErrorReporting{Provider: adaptererrorreporting.NoopProvider},
		},
		{
			name: "sentry requires dsn",
			config: ErrorReporting{
				Provider: adaptererrorreporting.SentryProvider,
				Sentry:   SentryErrorReporting{SampleRate: 1},
			},
			wantErr: "sentry.dsn",
		},
		{
			name: "sentry requires positive sample rate",
			config: ErrorReporting{
				Provider: adaptererrorreporting.SentryProvider,
				Sentry:   SentryErrorReporting{DSN: "https://public@example.invalid/1"},
			},
			wantErr: "sample_rate",
		},
		{
			name: "sentry accepts valid configuration",
			config: ErrorReporting{
				Provider: adaptererrorreporting.SentryProvider,
				Sentry: SentryErrorReporting{
					DSN:        "https://public@example.invalid/1",
					SampleRate: 1,
				},
			},
		},
		{
			name:    "unknown provider rejected",
			config:  ErrorReporting{Provider: "unknown"},
			wantErr: "unsupported value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.config.validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}

			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestValidateHTTPSURL(t *testing.T) {
	t.Parallel()

	require.NoError(t, validateHTTPSURL("external.financial_core.payment_return_url", "https://bnpl.example/return"))

	err := validateHTTPSURL("external.financial_core.payment_return_url", "http://bnpl.example/return")
	require.Error(t, err)
	require.Contains(t, err.Error(), "HTTPS")
}

func TestConfig_Validate_ProductionRequiresIssuanceScrub(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.Runtime.Environment = "production"
	cfg.External.FinancialCore.Provider = string(checkoutpaymentext.GRPCProvider)
	cfg.External.FinancialCore.GRPC.Address = "127.0.0.1:50052"
	cfg.External.FinancialCore.GRPC.Timeout = time.Second
	requireProductionIdentityAndNotification(cfg)
	cfg.External.CreditAssessment.Provider = string(creditassessmentext.CreditBureauProvider)
	cfg.Infrastructure.Logger.Provider = log.SlogProvider
	cfg.Infrastructure.Messaging = Messaging{
		Provider: messaging.KafkaProvider,
		Kafka: Kafka{
			Brokers:  []string{"localhost:9092"},
			ClientID: "bnpl",
			GroupID:  "bnpl",
			Topics: Topics{
				PaymentResults:                                   "payment.results",
				AuditEvents:                                      "audit.events",
				PaymentApplicationResults:                        "payment.application.results",
				InstallmentPaymentRequests:                       "installment.payment.requests",
				InstallmentPaymentSessions:                       "installment.payment.sessions",
				InstallmentPaymentReconciliationRequests:         "installment.payment.reconciliation.requests",
				InstallmentPaymentReconciliationResults:          "installment.payment.reconciliation.results",
				InstallmentPaymentReconciliationAcknowledgements: "installment.payment.reconciliation.acknowledgements",
				NotificationSMSRequests: "notification.sms.requests",
			},
		},
	}
	cfg.Infrastructure.Task = enabledSagaExecutorTask(taskadapter.InProcessProvider)
	cfg.Infrastructure.CircuitBreaker.Provider = circuitbreaker.GoBreakerProvider
	require.NoError(t, cfg.Validate())

	cfg.Infrastructure.Task.Scheduled.MerchantTerminalIssuanceScrub.Enabled = false
	err := cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "merchant_terminal_issuance_scrub.enabled must be true in production")
}

func TestConfig_Validate_DevelopmentAllowsDisabledIssuanceScrub(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.Runtime.Environment = "development"
	cfg.Infrastructure.Task.Scheduled.MerchantTerminalIssuanceScrub.Enabled = false
	require.NoError(t, cfg.Validate())
}

func TestConfig_Validate_ProductionNoopGuards(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.Runtime.Environment = "production"
	cfg.External.FinancialCore.Provider = string(checkoutpaymentext.GRPCProvider)
	cfg.External.FinancialCore.GRPC.Address = "127.0.0.1:50052"
	cfg.External.FinancialCore.GRPC.Timeout = time.Second
	requireProductionIdentityAndNotification(cfg)
	cfg.External.CreditAssessment.Provider = string(creditassessmentext.CreditBureauProvider)
	cfg.Infrastructure.Logger.Provider = log.SlogProvider
	cfg.Infrastructure.Messaging = Messaging{
		Provider: messaging.KafkaProvider,
		Kafka: Kafka{
			Brokers:  []string{"localhost:9092"},
			ClientID: "bnpl",
			GroupID:  "bnpl",
			Topics: Topics{
				PaymentResults:                                   "payment.results",
				AuditEvents:                                      "audit.events",
				PaymentApplicationResults:                        "payment.application.results",
				InstallmentPaymentRequests:                       "installment.payment.requests",
				InstallmentPaymentSessions:                       "installment.payment.sessions",
				InstallmentPaymentReconciliationRequests:         "installment.payment.reconciliation.requests",
				InstallmentPaymentReconciliationResults:          "installment.payment.reconciliation.results",
				InstallmentPaymentReconciliationAcknowledgements: "installment.payment.reconciliation.acknowledgements",
				NotificationSMSRequests: "notification.sms.requests",
			},
		},
	}
	cfg.Infrastructure.Task = enabledSagaExecutorTask(taskadapter.InProcessProvider)
	cfg.Infrastructure.CircuitBreaker.Provider = circuitbreaker.GoBreakerProvider

	require.NoError(t, cfg.Validate())

	cfg.Infrastructure.Messaging.Provider = messaging.NoopProvider
	err := cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), `messaging provider "noop" is not allowed in production`)

	cfg.Infrastructure.Messaging.Provider = messaging.KafkaProvider
	cfg.Infrastructure.Messaging.Kafka = Kafka{
		Brokers:  []string{"localhost:9092"},
		ClientID: "bnpl",
		GroupID:  "bnpl",
		Topics: Topics{
			PaymentResults:                                   "payment.results",
			AuditEvents:                                      "audit.events",
			PaymentApplicationResults:                        "payment.application.results",
			InstallmentPaymentRequests:                       "installment.payment.requests",
			InstallmentPaymentSessions:                       "installment.payment.sessions",
			InstallmentPaymentReconciliationRequests:         "installment.payment.reconciliation.requests",
			InstallmentPaymentReconciliationResults:          "installment.payment.reconciliation.results",
			InstallmentPaymentReconciliationAcknowledgements: "installment.payment.reconciliation.acknowledgements",
				NotificationSMSRequests: "notification.sms.requests",
		},
	}
	cfg.Infrastructure.Task.Provider = taskadapter.NoopProvider
	err = cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), `task provider "noop" is not allowed in production`)
}

func TestConfig_Validate_PaymentReturnURLRequired(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.External.FinancialCore.PaymentReturnURL = ""

	err := cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "external.financial_core.payment_return_url")
}

func TestConfig_Validate_ProductionSwaggerForbidden(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.Runtime.Environment = "production"
	cfg.Gin.Swagger.Enabled = true

	err := cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "gin.swagger.enabled must be false in production")
}

func TestConfig_Validate_ProductionCircuitBreakerNoopForbidden(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.Runtime.Environment = "production"
	cfg.Infrastructure.Logger.Provider = log.SlogProvider
	cfg.Infrastructure.Messaging.Provider = messaging.KafkaProvider
	cfg.Infrastructure.Messaging.Kafka = Kafka{
		Brokers: []string{"localhost:9092"}, ClientID: "bnpl", GroupID: "bnpl",
		Topics: Topics{
			PaymentResults: "payment.results", AuditEvents: "audit.events",
			PaymentApplicationResults:                        "payment.application.results",
			InstallmentPaymentRequests:                       "installment.payment.requests",
			InstallmentPaymentSessions:                       "installment.payment.sessions",
			InstallmentPaymentReconciliationRequests:         "installment.payment.reconciliation.requests",
			InstallmentPaymentReconciliationResults:          "installment.payment.reconciliation.results",
			InstallmentPaymentReconciliationAcknowledgements: "installment.payment.reconciliation.acknowledgements",
				NotificationSMSRequests: "notification.sms.requests",
		},
	}
	cfg.Infrastructure.Task = enabledSagaExecutorTask(taskadapter.InProcessProvider)
	cfg.External.CreditAssessment.Provider = string(creditassessmentext.CreditBureauProvider)
	cfg.External.FinancialCore.Provider = string(checkoutpaymentext.GRPCProvider)
	cfg.External.FinancialCore.GRPC.Address = "127.0.0.1:50052"
	cfg.External.FinancialCore.GRPC.Timeout = time.Second
	requireProductionIdentityAndNotification(cfg)
	cfg.Infrastructure.CircuitBreaker.Provider = circuitbreaker.NoopProvider

	err := cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), `circuit breaker provider "noop" is not allowed in production`)
}

func TestConfig_Validate_ProductionHTTPClientNoopForbidden(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.Runtime.Environment = "production"
	cfg.Infrastructure.Logger.Provider = log.SlogProvider
	cfg.Infrastructure.HTTPClient.Provider = adapterhttpclient.NoopProvider

	err := cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), `http client provider "noop" is not allowed in production`)
}

func TestConfig_Validate_ProductionWebhookDispatcherNoopForbidden(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.Runtime.Environment = "production"
	cfg.Infrastructure.Logger.Provider = log.SlogProvider
	cfg.Infrastructure.Messaging.Provider = messaging.KafkaProvider
	cfg.Infrastructure.Messaging.Kafka = Kafka{
		Brokers: []string{"localhost:9092"}, ClientID: "bnpl", GroupID: "bnpl",
		Topics: Topics{
			PaymentResults: "payment.results", AuditEvents: "audit.events",
			PaymentApplicationResults:                        "payment.application.results",
			InstallmentPaymentRequests:                       "installment.payment.requests",
			InstallmentPaymentSessions:                       "installment.payment.sessions",
			InstallmentPaymentReconciliationRequests:         "installment.payment.reconciliation.requests",
			InstallmentPaymentReconciliationResults:          "installment.payment.reconciliation.results",
			InstallmentPaymentReconciliationAcknowledgements: "installment.payment.reconciliation.acknowledgements",
				NotificationSMSRequests: "notification.sms.requests",
		},
	}
	cfg.Infrastructure.Task = enabledSagaExecutorTask(taskadapter.InProcessProvider)
	cfg.Infrastructure.CircuitBreaker.Provider = circuitbreaker.GoBreakerProvider
	cfg.External.CreditAssessment.Provider = string(creditassessmentext.CreditBureauProvider)
	cfg.External.FinancialCore.Provider = string(checkoutpaymentext.GRPCProvider)
	cfg.External.FinancialCore.GRPC.Address = "127.0.0.1:50052"
	cfg.External.FinancialCore.GRPC.Timeout = time.Second
	requireProductionIdentityAndNotification(cfg)
	cfg.External.WebhookDispatcher.Provider = string(merchantwebhook.NoopProvider)

	err := cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), `external.webhook_dispatcher provider "noop" is not allowed in production`)
}

func TestExternalNotificationValidate(t *testing.T) {
	t.Parallel()

	t.Run("unsupported provider", func(t *testing.T) {
		t.Parallel()

		err := ExternalNotification{Provider: "unknown"}.validate(model.DevelopmentRuntimeEnvironment)
		require.Error(t, err)
		require.Contains(t, err.Error(), "unsupported value")
	})

	t.Run("grpc requires address", func(t *testing.T) {
		t.Parallel()

		err := ExternalNotification{
			Provider: string(notificationext.GRPCProvider),
			GRPC:     ExternalNotificationGRPC{Timeout: time.Second},
		}.validate(model.DevelopmentRuntimeEnvironment)
		require.Error(t, err)
		require.Contains(t, err.Error(), "external.notification.grpc.address")
	})

	t.Run("grpc requires positive timeout", func(t *testing.T) {
		t.Parallel()

		err := ExternalNotification{
			Provider: string(notificationext.GRPCProvider),
			GRPC: ExternalNotificationGRPC{
				Address: "127.0.0.1:50054",
			},
		}.validate(model.DevelopmentRuntimeEnvironment)
		require.Error(t, err)
		require.Contains(t, err.Error(), "external.notification.grpc.timeout")
	})

	t.Run("noop forbidden in production", func(t *testing.T) {
		t.Parallel()

		err := ExternalNotification{
			Provider: string(notificationext.NoopProvider),
		}.validate(model.ProductionRuntimeEnvironment)
		require.Error(t, err)
		require.Contains(t, err.Error(), `provider "noop" is not allowed in production`)
	})

	t.Run("insecure forbidden in production", func(t *testing.T) {
		t.Parallel()

		err := ExternalNotification{
			Provider: string(notificationext.GRPCProvider),
			GRPC: ExternalNotificationGRPC{
				Address:  "127.0.0.1:50054",
				Timeout:  time.Second,
				Insecure: true,
			},
		}.validate(model.ProductionRuntimeEnvironment)
		require.Error(t, err)
		require.Contains(t, err.Error(), "external.notification.grpc.insecure must be false in production")
	})
}

func TestHTTPClientValidate_StandardRequiresPositiveCapacityAndTimeouts(t *testing.T) {
	t.Parallel()

	httpClient := validHTTPClient()
	httpClient.Standard.MaxInFlight = 0

	err := httpClient.validate(model.DevelopmentRuntimeEnvironment)
	require.Error(t, err)
	require.Contains(t, err.Error(), "max_in_flight")

	httpClient = validHTTPClient()
	httpClient.Standard.QueueWaitTimeout = 0
	err = httpClient.validate(model.DevelopmentRuntimeEnvironment)
	require.Error(t, err)
	require.Contains(t, err.Error(), "queue_wait_timeout")
}

func TestConfig_Validate_ProductionFinancialCoreInsecureGRPCForbidden(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.Runtime.Environment = "production"
	cfg.Infrastructure.Logger.Provider = log.SlogProvider
	cfg.Infrastructure.Messaging = Messaging{
		Provider: messaging.KafkaProvider,
		Kafka: Kafka{
			Brokers: []string{"localhost:9092"}, ClientID: "bnpl", GroupID: "bnpl",
			Topics: Topics{
				PaymentResults: "payment.results", AuditEvents: "audit.events",
				PaymentApplicationResults:                        "payment.application.results",
				InstallmentPaymentRequests:                       "installment.payment.requests",
				InstallmentPaymentSessions:                       "installment.payment.sessions",
				InstallmentPaymentReconciliationRequests:         "installment.payment.reconciliation.requests",
				InstallmentPaymentReconciliationResults:          "installment.payment.reconciliation.results",
				InstallmentPaymentReconciliationAcknowledgements: "installment.payment.reconciliation.acknowledgements",
				NotificationSMSRequests: "notification.sms.requests",
			},
		},
	}
	cfg.Infrastructure.Task = enabledSagaExecutorTask(taskadapter.InProcessProvider)
	cfg.Infrastructure.CircuitBreaker.Provider = circuitbreaker.GoBreakerProvider
	cfg.External.CreditAssessment.Provider = string(creditassessmentext.CreditBureauProvider)
	cfg.External.FinancialCore.Provider = string(checkoutpaymentext.GRPCProvider)
	cfg.External.FinancialCore.GRPC.Address = "financial-core:50052"
	cfg.External.FinancialCore.GRPC.Timeout = time.Second
	cfg.External.FinancialCore.GRPC.Insecure = true

	err := cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "external.financial_core.grpc.insecure must be false in production")
}

func TestConfig_Validate_ProductionDatabaseQueryLoggingForbidden(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.Runtime.Environment = "production"
	cfg.Database.QueryLogging = QueryLogging{
		Enabled:          true,
		IncludeArguments: true,
	}

	err := cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "database.query_logging.enabled must be false in production")
}

func TestConfig_Validate_DatabaseQueryLoggingRequiresRealLogger(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.Database.QueryLogging.Enabled = true
	cfg.Infrastructure.Logger.Provider = log.NoopProvider

	err := cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "database.query_logging.enabled requires a non-noop infrastructure logger")

	cfg.Infrastructure.Logger.Provider = log.SlogProvider
	require.NoError(t, cfg.Validate())
}

func TestConfig_Validate_KafkaNewestInitialOffsetForbiddenInProduction(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.Runtime.Environment = "production"
	cfg.Infrastructure.Logger.Provider = log.SlogProvider
	cfg.Infrastructure.Messaging = Messaging{
		Provider: messaging.KafkaProvider,
		Kafka: Kafka{
			Brokers:  []string{"localhost:9092"},
			ClientID: "bnpl",
			GroupID:  "bnpl",
			Topics: Topics{
				PaymentResults:                                   "payment.results",
				AuditEvents:                                      "audit.events",
				PaymentApplicationResults:                        "payment.application.results",
				InstallmentPaymentRequests:                       "installment.payment.requests",
				InstallmentPaymentSessions:                       "installment.payment.sessions",
				InstallmentPaymentReconciliationRequests:         "installment.payment.reconciliation.requests",
				InstallmentPaymentReconciliationResults:          "installment.payment.reconciliation.results",
				InstallmentPaymentReconciliationAcknowledgements: "installment.payment.reconciliation.acknowledgements",
				NotificationSMSRequests: "notification.sms.requests",
			},
			Consumer: Consumer{
				InitialOffset:            "newest",
				AllowNewestInitialOffset: true,
			},
		},
	}
	cfg.Infrastructure.Task = enabledSagaExecutorTask(taskadapter.InProcessProvider)
	cfg.Infrastructure.CircuitBreaker.Provider = circuitbreaker.GoBreakerProvider
	cfg.External.CreditAssessment.Provider = string(creditassessmentext.CreditBureauProvider)
	cfg.External.FinancialCore.Provider = string(checkoutpaymentext.GRPCProvider)
	cfg.External.FinancialCore.GRPC.Address = "financial-core:50052"
	cfg.External.FinancialCore.GRPC.Timeout = time.Second

	err := cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "initial_offset newest is not allowed in production")
}

func TestConfig_Validate_KafkaNewestInitialOffsetRequiresExplicitAllow(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.Infrastructure.Messaging = Messaging{
		Provider: messaging.KafkaProvider,
		Kafka: Kafka{
			Brokers:  []string{"localhost:9092"},
			ClientID: "bnpl",
			GroupID:  "bnpl",
			Topics: Topics{
				PaymentResults:                                   "payment.results",
				AuditEvents:                                      "audit.events",
				PaymentApplicationResults:                        "payment.application.results",
				InstallmentPaymentRequests:                       "installment.payment.requests",
				InstallmentPaymentSessions:                       "installment.payment.sessions",
				InstallmentPaymentReconciliationRequests:         "installment.payment.reconciliation.requests",
				InstallmentPaymentReconciliationResults:          "installment.payment.reconciliation.results",
				InstallmentPaymentReconciliationAcknowledgements: "installment.payment.reconciliation.acknowledgements",
				NotificationSMSRequests: "notification.sms.requests",
			},
			Consumer: Consumer{InitialOffset: "newest"},
		},
	}

	err := cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "allow_newest_initial_offset=true")

	cfg.Infrastructure.Messaging.Kafka.Consumer.AllowNewestInitialOffset = true
	require.NoError(t, cfg.Validate())
}

func TestTaskValidate_TemporalRequiresConnectionAndActivityTimeout(t *testing.T) {
	t.Parallel()

	taskCfg := enabledSagaExecutorTask(taskadapter.TemporalProvider)

	err := taskCfg.validate(model.DevelopmentRuntimeEnvironment)
	require.Error(t, err)
	require.Contains(t, err.Error(), "infrastructure.task.temporal.address")

	taskCfg.Temporal = Temporal{
		Address:                     "127.0.0.1:7233",
		Namespace:                   "default",
		TaskQueue:                   "bnpl",
		ActivityStartToCloseTimeout: time.Hour,
	}
	require.NoError(t, taskCfg.validate(model.DevelopmentRuntimeEnvironment))
}

func TestTaskValidate_CompositeRequiresExecutionForEnabledTasks(t *testing.T) {
	t.Parallel()

	taskCfg := enabledSagaExecutorTask(taskadapter.CompositeProvider)
	taskCfg.Temporal = Temporal{
		Address:                     "127.0.0.1:7233",
		Namespace:                   "default",
		TaskQueue:                   "bnpl",
		ActivityStartToCloseTimeout: time.Hour,
	}

	err := taskCfg.validate(model.DevelopmentRuntimeEnvironment)
	require.Error(t, err)
	require.Contains(t, err.Error(), ".execution")
	require.Contains(t, err.Error(), "value is required when provider=composite")

	taskCfg.Scheduled.SagaExecutor.Execution = coretask.ExecutionModeTemporal
	taskCfg.Scheduled.MerchantTerminalIssuanceScrub.Execution = coretask.ExecutionModeTemporal
	require.NoError(t, taskCfg.validate(model.DevelopmentRuntimeEnvironment))
}

func TestTaskValidate_StandaloneProviderRejectsExecutionRouting(t *testing.T) {
	t.Parallel()

	taskCfg := enabledSagaExecutorTask(taskadapter.InProcessProvider)
	taskCfg.Scheduled.SagaExecutor.Execution = coretask.ExecutionModeInProcess

	err := taskCfg.validate(model.DevelopmentRuntimeEnvironment)
	require.Error(t, err)
	require.Contains(t, err.Error(), "must be empty when provider=inprocess")
}
