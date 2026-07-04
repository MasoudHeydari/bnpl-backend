package bootstrap

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/adapter/config"
	checkoutpaymentext "github.com/parstis/bnpl-backend/pkg/adapter/external/checkoutpayment"
	creditassessmentext "github.com/parstis/bnpl-backend/pkg/adapter/external/creditassessment"
	identityext "github.com/parstis/bnpl-backend/pkg/adapter/external/identity"
	"github.com/parstis/bnpl-backend/pkg/adapter/external/merchantwebhook"
	notificationext "github.com/parstis/bnpl-backend/pkg/adapter/external/notification"
	adaptercache "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/cache"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/circuitbreaker"
	adapterhttpclient "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/httpclient"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/messaging"
	taskadapter "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/task"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/task"
)

func noopTestConfig() *config.Config {
	return &config.Config{
		Domain: config.Domain{
			SupportedJurisdictions: []model.Jurisdiction{model.IRJurisdiction},
		},
		Runtime: config.Runtime{Environment: "development"},
		Usecases: config.Usecases{
			CheckoutSession: config.CheckoutSession{
				PublicOrigin: "https://bnpl.example",
				PublicOriginPolicy: config.PublicOriginPolicyConfig{
					AllowedHosts: []string{"bnpl.example"},
				},
				TTL:                        30 * time.Minute,
				ExpiryBatchSize:            100,
				LifecycleEventsDestination: "bnpl.merchant.checkout.events.v1",
				WebhookSecretEncryptionKeys: config.KeyEntries{
					{ID: "w1", Secret: model.NewSecret("ZGV2LW9ubHktd2ViaG9vay1lbmNyeXB0aW9uLWtleSE=")},
				},
				CustomerPhoneEncryptionKeys: config.KeyEntries{
					{ID: "p1", Secret: model.NewSecret("ZGV2LW9ubHktY3VzdG9tZXItcGhvbmUta2V5LTAxMjM=")},
				},
				OTPPepperKeys: config.KeyEntries{
					{ID: "o1", Secret: model.NewSecret("test-checkout-otp-pepper-0123456789ab")},
				},
			},
			OutboxPublisher: config.OutboxPublisher{
				WorkerID:               "worker-1",
				BatchSize:              5,
				RetryBaseDelay:         time.Second,
				RetryMaxDelay:          time.Minute,
				ProcessingLeaseTimeout: 5 * time.Minute,
			},
			Health: config.Health{
				Enabled: true,
			},
			Installment: config.Installment{
				MarkDueBatchSize:                      100,
				MarkOverdueBatchSize:                  100,
				InstallmentPaymentRequestsDestination: "bnpl.installment_payment_requests",
			},
			PaymentAttemptReconciliation: config.PaymentAttemptReconciliation{
				BatchSize:            100,
				CreatedTimeout:       5 * time.Minute,
				SubmittedExpiryGrace: time.Minute,
				RetryBaseDelay:       time.Second,
				RetryMaxDelay:        time.Minute,
			},
			Contract: config.Contract{
				PaymentResultProcessingGracePeriod: 5 * time.Minute,
				ExpiryBatchSize:                    100,
			},
			MerchantTerminal: config.MerchantTerminal{
				APIKeyPepper: model.NewSecret("test-merchant-terminal-api-key-pepper-32b"),
				Rotation: config.MerchantTerminalRotation{
					DefaultGracePeriod: 24 * time.Hour,
					MaxGracePeriod:     7 * 24 * time.Hour,
				},
				IssuanceDeliveryTTL:           24 * time.Hour,
				IssuanceEncryptionActiveKeyID: "m1",
				IssuanceEncryptionKeys: config.KeyEntries{
					{ID: "m1", Secret: model.NewSecret("dGVzdC1tZXJjaGFudC1hdXRoLWlzc3VhbmNlLWtleSE=")},
				},
			},
		},
		Infrastructure: config.Infrastructure{
			Clock: config.Clock{
				Location:         "UTC",
				BusinessLocation: "UTC",
			},
			Logger: config.Logger{
				Provider: log.NoopProvider,
			},
			HTTPClient: config.HTTPClient{
				Provider: adapterhttpclient.NoopProvider,
			},
			Cache: config.Cache{
				Provider: adaptercache.NoopProvider,
			},
			Messaging: config.Messaging{
				Provider: messaging.NoopProvider,
				Kafka: config.Kafka{
					Topics: config.Topics{
						AuditEvents:                                      "bnpl.audit.events.v1",
						PaymentApplicationResults:                        "financial_core.payment_application_results",
						InstallmentPaymentReconciliationRequests:         "bnpl.installment_payment_reconciliation_requests",
						InstallmentPaymentReconciliationResults:          "financial_core.installment_payment_reconciliation_results",
						InstallmentPaymentReconciliationAcknowledgements: "bnpl.installment_payment_reconciliation_acknowledgements",
					},
				},
			},
			CircuitBreaker: config.CircuitBreaker{
				Provider: circuitbreaker.NoopProvider,
			},
			Task: config.Task{
				Provider: taskadapter.NoopProvider,
				Scheduled: config.ScheduledTasks{
					RuntimeGoroutineMetrics: config.TaskDefinition{
						Enabled: false,
						Handler: "runtime_goroutine_metrics",
						Trigger: config.TaskTrigger{
							Type: task.TriggerManual,
						},
					},
					OutboxPublisher: config.TaskDefinition{
						Enabled: false,
						Handler: "outbox_publisher",
						Trigger: config.TaskTrigger{
							Type:     task.TriggerInterval,
							Interval: "5s",
						},
					},
					InstallmentOfferExpiry: config.TaskDefinition{
						Enabled: false,
						Handler: "installment_offer_expiry",
						Trigger: config.TaskTrigger{
							Type: task.TriggerManual,
						},
					},
					ContractPaymentPendingExpiry: config.TaskDefinition{
						Enabled: false,
						Handler: "contract_payment_pending_expiry",
						Trigger: config.TaskTrigger{
							Type: task.TriggerManual,
						},
					},
					CheckoutSessionExpiry: config.TaskDefinition{
						Enabled: false,
						Handler: "checkout_session_expiry",
						Trigger: config.TaskTrigger{
							Type: task.TriggerManual,
						},
					},
					InstallmentMarkDue: config.TaskDefinition{
						Enabled: false,
						Handler: "installment_mark_due",
						Trigger: config.TaskTrigger{
							Type: task.TriggerManual,
						},
					},
					InstallmentMarkOverdue: config.TaskDefinition{
						Enabled: false,
						Handler: "installment_mark_overdue",
						Trigger: config.TaskTrigger{
							Type: task.TriggerManual,
						},
					},
					PaymentAttemptReconciliation: config.TaskDefinition{
						Enabled: false,
						Handler: "payment_attempt_reconciliation",
						Trigger: config.TaskTrigger{
							Type: task.TriggerManual,
						},
					},
				},
			},
		},
		External: config.External{
			CreditAssessment: config.ExternalCreditAssessment{
				Provider: string(creditassessmentext.NoopProvider),
			},
			FinancialCore: config.ExternalFinancialCore{
				Provider:         string(checkoutpaymentext.NoopProvider),
				PaymentReturnURL: "https://bnpl.example/payments/return",
			},
			Identity: config.ExternalIdentity{
				Provider: string(identityext.NoopProvider),
			},
			Notification: config.ExternalNotification{
				Provider: string(notificationext.NoopProvider),
			},
			WebhookDispatcher: config.ExternalWebhookDispatcher{
				Provider: string(merchantwebhook.NoopProvider),
			},
		},
	}
}

func testAppWithInfrastructure(t *testing.T, cfg *config.Config) *App {
	t.Helper()

	app := &App{
		Config: cfg,
		Pool:   mock.NewPool(t),
		Repos:  NewRepos(),
	}

	infrastructure, err := app.NewInfrastructures(context.Background())
	require.NoError(t, err)

	app.Infrastructure = infrastructure

	require.NoError(t, app.wireDependencyChecker())

	external, err := app.NewExternal()
	require.NoError(t, err)
	app.External = external

	return app
}

func TestNewInfrastructures_NoopConfig(t *testing.T) {
	t.Parallel()

	app := testAppWithInfrastructure(t, noopTestConfig())
	infra := app.Infrastructure

	require.NotNil(t, infra.DependencyChecker)
	require.NotNil(t, infra.Trace)
	require.NotNil(t, infra.Clock)
	require.NotNil(t, infra.Logger)
	require.NotNil(t, infra.HTTPClient)
	require.NotNil(t, infra.Cache)
	require.NotNil(t, infra.Publisher)
	require.NotNil(t, infra.Subscriber)
	require.NotNil(t, infra.LifecycleState)
	require.NotNil(t, infra.CircuitBreaker)
	require.NotNil(t, infra.Task)
}
