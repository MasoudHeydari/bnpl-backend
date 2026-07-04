package bootstrap

import (
	"context"
	"fmt"
	"strings"

	"github.com/parstis/bnpl-backend/pkg/core/policy/collections"
	collectionsv1 "github.com/parstis/bnpl-backend/pkg/core/policy/collections/v1"
	collectionsv2 "github.com/parstis/bnpl-backend/pkg/core/policy/collections/v2"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	checkoutcatalog "github.com/parstis/bnpl-backend/pkg/core/policy/checkout/catalog"
	initialcreditcatalog "github.com/parstis/bnpl-backend/pkg/core/policy/initialcreditlimit/catalog"
	"github.com/parstis/bnpl-backend/pkg/core/policy/shortlinkdestination"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/checkoutsessionuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/collectionsuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/contractuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/customeraccountuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/customercredituc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/decisionuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/financialcorepaymentreconciliationuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/financialcorepaymentresultuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/financialcorepaymentsessionuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/healthcheckuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/installmentlifecycleuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/installmentrepaymentuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/merchantterminaluc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/merchantuc"
	otpusecase "github.com/parstis/bnpl-backend/pkg/core/usecase/otp"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/outboxpublisheruc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/paymentattemptreconciliationuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/repaymentreminderuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/sagaexecutoruc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/shortlinkuc"
)

// UseCases holds the application use cases assembled for App.
//
// Each field is a core use-case port wired with repositories, external clients,
// and infrastructure from this composition root. HTTP handlers and background
// jobs depend on these instances; they must not construct use cases themselves.
type UseCases struct {

	// OutboxPublisher publishes due outbox messages.
	OutboxPublisher *outboxpublisheruc.UseCase

	// Health serves liveness/readiness probes.
	Health *healthcheckuc.UseCase

	// Decision creates checkout financing decisions.
	Decision *decisionuc.UseCase

	// CheckoutSession coordinates merchant checkout handoffs and identity binding.
	CheckoutSession *checkoutsessionuc.UseCase

	// OTP issues and verifies reusable phone OTP challenges.
	OTP *otpusecase.UseCase

	// InstallmentLifecycle advances installment due/overdue state.
	InstallmentLifecycle *installmentlifecycleuc.UseCase

	// InstallmentRepayment starts customer installment repayment attempts.
	InstallmentRepayment *installmentrepaymentuc.UseCase

	// FinancialCorePaymentResult applies inbound payment-result messages.
	FinancialCorePaymentResult *financialcorepaymentresultuc.UseCase

	// FinancialCorePaymentSession applies inbound payment-session messages.
	FinancialCorePaymentSession *financialcorepaymentsessionuc.UseCase

	// FinancialCorePaymentReconciliation applies inbound reconciliation results.
	FinancialCorePaymentReconciliation *financialcorepaymentreconciliationuc.UseCase

	// PaymentAttemptReconciliation schedules and processes attempt reconciliation.
	PaymentAttemptReconciliation *paymentattemptreconciliationuc.UseCase

	// SagaExecutor resumes durable Saga instances from the background job.
	SagaExecutor *sagaexecutoruc.UseCase

	// Contract creates, activates, and cancels BNPL contracts.
	Contract *contractuc.UseCase

	// Merchant serves merchant-facing queries.
	Merchant *merchantuc.UseCase

	// MerchantTerminal authenticates external partner terminals and manages
	// their credentials.
	MerchantTerminal *merchantterminaluc.UseCase

	// CustomerAccount serves customer account views.
	CustomerAccount *customeraccountuc.UseCase

	// CustomerCredit assigns and summarizes customer credit limits.
	CustomerCredit *customercredituc.UseCase

	// ShortLink creates and resolves modular opaque URL short links.
	ShortLink *shortlinkuc.UseCase

	// RepaymentReminder dispatches repayment reminder SMS requests via outbox.
	RepaymentReminder *repaymentreminderuc.UseCase

	// Collections assesses default entry/recovery and credit-usage restrictions.
	Collections *collectionsuc.UseCase
}

// NewUseCases constructs the use cases required by App.
//
// Domain-owned Saga definitions are created here and injected both into the
// generic Saga executor and into the use cases that start them, so start-path
// and resume-path share one definition instance.
func (a *App) NewUseCases() (UseCases, error) {
	const op = "bootstrap.App.NewUseCases"

	// Webhook routing publisher is owned by External; OutboxPublisher only
	// consumes the already-constructed messaging.Publisher.
	outboxPublisherUseCase := a.Config.NewOutboxPublisherUseCase(
		a.Pool,
		a.Repos.Outbox,
		a.External.WebhookPublisher,
		a.Infrastructure.Clock,
		a.Infrastructure.Logger,
	)
	if err := outboxPublisherUseCase.Validate(); err != nil {
		return UseCases{}, fmt.Errorf("%s: initialize outbox publisher use case: %w", op, err)
	}

	healthUseCase := a.Config.NewHealthUseCase(
		a.Infrastructure.LifecycleState,
		a.Infrastructure.DependencyChecker,
	)

	checkoutPolicies, err := checkoutcatalog.NewRegistry()
	if err != nil {
		return UseCases{}, fmt.Errorf("%s: initialize checkout policy registry: %w", op, err)
	}

	// One immutable origination-capability instance for Decision, Contract,
	// CheckoutSession, and Phone Registry. Factories must not reconstruct it.
	jurisdictions, err := a.Config.NewJurisdictionCapabilities()
	if err != nil {
		return UseCases{}, fmt.Errorf("%s: initialize jurisdiction capabilities: %w", op, err)
	}

	collectionsBatchSize := a.Config.Usecases.Collections.BatchSize
	if collectionsBatchSize <= 0 {
		collectionsBatchSize = 100
	}

	collectionsPolicies, err := collections.NewRegistry(collectionsv2.New(a.Infrastructure.Clock.BusinessLocation()), collectionsv1.New())
	if err != nil {
		return UseCases{}, err
	}
	collectionsUseCase, err := collectionsuc.New(
		a.Pool,
		a.Repos.CustomerProfile,
		a.Repos.Installment,
		a.Repos.Decision,
		a.Repos.DefaultEpisode,
		a.Repos.CustomerRestriction,
		a.Repos.CollectionsCase,
		a.Repos.Outbox,
		a.Infrastructure.Clock,
		a.Infrastructure.DomainUUIDGenerator,
		a.Infrastructure.FingerprintHasher,
		a.Infrastructure.Metrics,
		a.Config.AuditEventsTopic(),
		collectionsBatchSize,
		collectionsPolicies,
	)
	if err != nil {
		return UseCases{}, fmt.Errorf("%s: initialize collections use case: %w", op, err)
	}
	collectionsUseCase.WithAssessmentFailureObserver(func(ctx context.Context, customerID model.CustomerID, err error) {
		a.Infrastructure.Logger.With("customer_id", customerID.String(), "error", err.Error()).Error("collections customer assessment failed")
	})

	decisionUseCase, err := a.Config.NewDecisionUseCase(
		a.Pool,
		a.Repos.Decision,
		a.Repos.CustomerProfile,
		a.Repos.CreditAccount,
		a.Repos.MerchantProfile,
		a.Repos.MerchantBranchProfile,
		a.Repos.CheckoutDecisionAllocation,
		a.Repos.PolicyAssignment,
		a.Repos.Installment,
		a.Repos.InstallmentCharge,
		a.Repos.Outbox,
		a.Infrastructure.Clock,
		a.Infrastructure.FingerprintHasher,
		checkoutPolicies,
		checkoutcatalog.DefaultRef(),
		a.Infrastructure.IdentifierGenerator,
		jurisdictions,
		a.Infrastructure.Metrics,
		collectionsUseCase,
	)
	if err != nil {
		return UseCases{}, fmt.Errorf("%s: initialize decision use case: %w", op, err)
	}

	installmentLifecycleUseCase, err := a.Config.NewInstallmentLifecycleUseCase(
		a.Pool,
		a.Repos.Installment,
		a.Repos.InstallmentCharge,
		a.Repos.Outbox,
		a.Infrastructure.Clock,
		a.Infrastructure.IdentifierGenerator,
		a.Infrastructure.Metrics,
	)
	if err != nil {
		return UseCases{}, fmt.Errorf("%s: initialize installment lifecycle use case: %w", op, err)
	}

	installmentRepaymentUseCase, err := a.Config.NewInstallmentRepaymentUseCase(
		a.Pool,
		a.Repos.CustomerProfile,
		a.Repos.MerchantProfile,
		a.Repos.Contract,
		a.Repos.Installment,
		a.Repos.InstallmentCharge,
		a.Repos.PaymentAttempt,
		a.Repos.Outbox,
		a.External.CheckoutPaymentClient,
		a.Infrastructure.Clock,
		a.Infrastructure.Logger,
		a.Infrastructure.IdentifierGenerator,
	)
	if err != nil {
		return UseCases{}, fmt.Errorf("%s: initialize installment repayment use case: %w", op, err)
	}

	// Saga definitions are owned by their domains and collected here into one
	// registry for the generic durable executor.
	//
	// To add a new Saga:
	//  1. Add Domain.newXxxSagaDefinition(...) (or Config.NewXxxSagaDefinition)
	//     that returns saga.Definition.
	//  2. Construct that definition in this composition root.
	//  3. Pass it to Config.NewSagaExecutorUseCase together with the existing
	//     definitions so the background executor can resume it.
	//  4. If a use case starts that Saga, inject the same definition instance
	//     plus repo.Saga and saga.Executor into that use case.
	// Do not teach NewSagaExecutorUseCase about domain-specific dependencies.
	cancellationSagaDefinition, err := a.Config.NewContractCancellationSagaDefinition(
		a.Pool,
		a.Repos.Contract,
		a.Repos.CreditAccount,
		a.External.ContractCancellationClient,
		a.Infrastructure.Clock,
	)
	if err != nil {
		return UseCases{}, fmt.Errorf("%s: initialize contract cancellation saga definition: %w", op, err)
	}
	cancellationSagaDefinition.(*contractuc.CancellationSagaDefinition).WithCustomerSynchronization(a.Repos.CustomerProfile)

	// Temporary executor covers cancellation until the confirmation definition
	// is constructed (it depends on contractUC). Both definitions share the
	// final registry used by background jobs and checkout Confirm.
	cancellationOnlyExecutor, err := a.Config.NewSagaExecutorUseCase(
		a.Pool,
		a.Repos.Saga,
		a.Infrastructure.Clock,
		cancellationSagaDefinition,
	)
	if err != nil {
		return UseCases{}, fmt.Errorf("%s: initialize cancellation saga executor: %w", op, err)
	}

	contractUseCase, err := a.Config.NewContractUseCase(
		a.Pool,
		a.Repos.CustomerProfile,
		a.Repos.MerchantProfile,
		a.Repos.CreditAccount,
		a.Repos.Decision,
		a.Repos.Contract,
		a.Repos.CheckoutDecisionAllocation,
		checkoutPolicies,
		a.Repos.Installment,
		a.Repos.InstallmentCharge,
		a.Repos.PaymentAttempt,
		a.External.CheckoutPaymentClient,
		a.External.ContractCancellationClient,
		a.Repos.Outbox,
		a.Infrastructure.Clock,
		a.Infrastructure.IdentifierGenerator,
		a.Repos.Saga,
		cancellationOnlyExecutor,
		cancellationSagaDefinition,
		jurisdictions,
		a.Infrastructure.Logger,
		a.Infrastructure.Metrics,
		collectionsUseCase,
	)
	if err != nil {
		return UseCases{}, fmt.Errorf("%s: initialize contract use case: %w", op, err)
	}

	webhookSecretBox, err := a.Config.Usecases.CheckoutSession.NewWebhookSecretBox()
	if err != nil {
		return UseCases{}, fmt.Errorf("%s: initialize webhook secret box: %w", op, err)
	}

	customerPhoneBox, err := a.Config.Usecases.CheckoutSession.NewCustomerPhoneBox()
	if err != nil {
		return UseCases{}, fmt.Errorf("%s: initialize customer phone box: %w", op, err)
	}

	confirmationSagaDefinition, err := a.Config.NewCheckoutConfirmationSagaDefinition(
		a.Pool,
		a.Repos.CheckoutSession,
		a.Repos.MerchantProfile,
		a.Repos.CustomerProfile,
		a.Repos.Contract,
		a.Repos.PaymentAttempt,
		a.Repos.Saga,
		contractUseCase,
		a.Repos.Outbox,
		a.Infrastructure.IdentifierGenerator,
		a.Infrastructure.Clock,
	)
	if err != nil {
		return UseCases{}, fmt.Errorf("%s: initialize checkout confirmation saga definition: %w", op, err)
	}

	sagaExecutorUseCase, err := a.Config.NewSagaExecutorUseCase(
		a.Pool,
		a.Repos.Saga,
		a.Infrastructure.Clock,
		cancellationSagaDefinition,
		confirmationSagaDefinition,
	)
	if err != nil {
		return UseCases{}, fmt.Errorf("%s: initialize saga executor use case: %w", op, err)
	}

	otpUseCase, err := a.Config.NewOTPUseCase(
		a.Pool,
		a.Repos.OTP,
		a.External.NotificationSubmitter,
		a.Infrastructure.Clock,
	)
	if err != nil {
		return UseCases{}, fmt.Errorf("%s: initialize otp use case: %w", op, err)
	}

	checkoutSessionUseCase, err := a.Config.NewCheckoutSessionUseCase(
		a.Pool,
		a.Repos.CheckoutSession,
		a.Repos.MerchantProfile,
		a.Repos.MerchantBranchProfile,
		a.Repos.CustomerProfile,
		a.Repos.Contract,
		a.Repos.PaymentAttempt,
		a.Repos.Installment,
		a.Repos.InstallmentCharge,
		a.Repos.Decision,
		a.Repos.CheckoutDecisionAllocation,
		checkoutPolicies,
		decisionUseCase,
		contractUseCase,
		a.Repos.Outbox,
		a.Infrastructure.IdentifierGenerator,
		webhookSecretBox,
		customerPhoneBox,
		a.Infrastructure.Clock,
		a.External.CustomerResolver,
		jurisdictions,
		otpUseCase,
		a.Repos.Saga,
		sagaExecutorUseCase,
		confirmationSagaDefinition,
	)
	if err != nil {
		return UseCases{}, fmt.Errorf("%s: initialize checkout session use case: %w", op, err)
	}

	financialCorePaymentResultUseCase, err := a.Config.NewFinancialCorePaymentResultUseCase(
		a.Pool,
		contractUseCase,
		checkoutSessionUseCase,
		a.External.CheckoutPaymentClient,
		a.Repos.Inbox,
		a.Repos.Outbox,
		a.Repos.PaymentAttempt,
		a.Repos.Contract,
		a.Repos.Installment,
		a.Repos.InstallmentCharge,
		a.Repos.CreditAccount,
		a.Infrastructure.IdentifierGenerator,
		a.Infrastructure.FingerprintHasher,
		a.Infrastructure.Clock,
		a.Infrastructure.Metrics,
	)
	if err != nil {
		return UseCases{}, fmt.Errorf("%s: initialize financial core payment result use case: %w", op, err)
	}
	financialCorePaymentResultUseCase.WithCollectionsCoordination(a.Repos.CustomerProfile, func(ctx context.Context, id model.CustomerID) error {
		_, err := collectionsUseCase.AssessCustomer(ctx, id)
		return err
	})

	financialCorePaymentSessionUseCase, err := a.Config.NewFinancialCorePaymentSessionUseCase(
		a.Pool,
		a.Repos.Inbox,
		a.Repos.PaymentAttempt,
		a.Repos.Outbox,
		a.Infrastructure.IdentifierGenerator,
		a.Infrastructure.FingerprintHasher,
		a.Infrastructure.Clock,
	)
	if err != nil {
		return UseCases{}, fmt.Errorf("%s: initialize financial core payment session use case: %w", op, err)
	}

	financialCorePaymentReconciliationUseCase, err := a.Config.NewFinancialCorePaymentReconciliationUseCase(
		a.Pool,
		financialCorePaymentResultUseCase,
		a.Repos.Inbox,
		a.Repos.Outbox,
		a.Repos.PaymentAttempt,
		a.Repos.Contract,
		a.Repos.Installment,
		a.Repos.InstallmentCharge,
		a.Repos.CreditAccount,
		a.Infrastructure.FingerprintHasher,
		a.Infrastructure.IdentifierGenerator,
		a.Infrastructure.Clock,
		a.Infrastructure.Logger,
	)
	if err != nil {
		return UseCases{}, fmt.Errorf("%s: initialize financial core payment reconciliation use case: %w", op, err)
	}

	paymentAttemptReconciliationUseCase, err := a.Config.NewPaymentAttemptReconciliationUseCase(
		a.Pool,
		a.Repos.PaymentAttempt,
		a.Repos.Contract,
		a.Repos.Outbox,
		a.Infrastructure.IdentifierGenerator,
		a.Infrastructure.Clock,
	)
	if err != nil {
		return UseCases{}, fmt.Errorf("%s: initialize payment attempt reconciliation use case: %w", op, err)
	}

	initialCreditLimitPolicies, err := initialcreditcatalog.NewRegistry()
	if err != nil {
		return UseCases{}, fmt.Errorf("%s: initialize initial credit limit policy registry: %w", op, err)
	}

	customerCreditUseCase, err := a.Config.NewCustomerCreditUseCase(
		a.Pool,
		a.Repos.CustomerProfile,
		a.Repos.CreditAccount,
		a.Repos.Decision,
		a.Repos.CustomerRestriction,
		a.Repos.Installment,
		a.Repos.DefaultEpisode,
		a.Repos.Outbox,
		a.Infrastructure.Clock,
		a.Infrastructure.FingerprintHasher,
		a.External.CreditAssessmentClient,
		a.Repos.PolicyAssignment,
		initialCreditLimitPolicies,
		initialcreditcatalog.DefaultRef(),
		a.Infrastructure.IdentifierGenerator,
		collectionsPolicies.Current,
	)
	if err != nil {
		return UseCases{}, fmt.Errorf("%s: initialize customer credit use case: %w", op, err)
	}

	merchantUseCase := merchantuc.New(
		a.Pool,
		a.Repos.MerchantProfile,
		a.Repos.MerchantBranchProfile,
		a.Repos.Contract,
	)

	merchantTerminalHasher, err := a.Config.NewMerchantTerminalHasher()
	if err != nil {
		return UseCases{}, fmt.Errorf("%s: initialize merchant terminal hasher: %w", op, err)
	}

	merchantTerminalSecretBox, err := a.Config.NewMerchantTerminalSecretBox()
	if err != nil {
		return UseCases{}, fmt.Errorf("%s: initialize merchant terminal secret box: %w", op, err)
	}

	merchantTerminalUseCase, err := a.Config.NewMerchantTerminalUseCase(
		a.Pool,
		a.Repos.MerchantProfile,
		a.Repos.MerchantTerminal,
		merchantTerminalHasher,
		merchantTerminalSecretBox,
		a.Infrastructure.Clock,
	)
	if err != nil {
		return UseCases{}, fmt.Errorf("%s: initialize merchant terminal use case: %w", op, err)
	}

	customerAccountUseCase, err := customeraccountuc.New(
		a.Pool,
		a.Repos.CustomerProfile,
		a.Repos.Contract,
		a.Repos.Installment,
		a.Repos.InstallmentCharge,
		a.Repos.PaymentAttempt,
		a.Infrastructure.Clock,
	)
	if err != nil {
		return UseCases{}, fmt.Errorf("%s: initialize customer account use case: %w", op, err)
	}

	destinationPolicy, err := shortlinkdestination.New(
		nonEmptyHosts(a.Config.Usecases.ShortLink.AllowedDestinationHosts),
	)
	if err != nil {
		return UseCases{}, fmt.Errorf("%s: initialize short link destination policy: %w", op, err)
	}

	shortLinkUseCase, err := shortlinkuc.New(
		a.Pool,
		a.Repos.ShortLink,
		destinationPolicy,
		a.Infrastructure.Clock,
		a.Infrastructure.DomainUUIDGenerator,
		a.Infrastructure.Metrics,
	)
	if err != nil {
		return UseCases{}, fmt.Errorf("%s: initialize short link use case: %w", op, err)
	}

	repaymentBatchSize := a.Config.Usecases.RepaymentReminder.BatchSize
	if repaymentBatchSize <= 0 {
		repaymentBatchSize = 200
	}

	publicHost := strings.TrimSpace(a.Config.Usecases.ShortLink.PublicHost)
	repaymentDestination := strings.TrimSpace(a.Config.Usecases.ShortLink.RepaymentDestinationURL)

	notificationTopic := a.Config.Infrastructure.Messaging.Kafka.Topics.NotificationSMSRequests
	if notificationTopic == "" {
		notificationTopic = "notification.sms.requests"
	}

	repaymentReminderUseCase, err := repaymentreminderuc.New(
		a.Pool,
		a.Repos.Installment,
		a.Repos.RepaymentReminder,
		a.Repos.PaymentAttempt,
		a.Repos.Outbox,
		shortLinkUseCase,
		a.Infrastructure.Clock,
		a.Infrastructure.DomainUUIDGenerator,
		a.Infrastructure.Metrics,
		notificationTopic,
		publicHost,
		repaymentDestination,
		repaymentBatchSize,
	)
	if err != nil {
		return UseCases{}, fmt.Errorf("%s: initialize repayment reminder use case: %w", op, err)
	}
	repaymentReminderUseCase.WithDefaultNotifications(collectionsUseCase)

	return UseCases{
		OutboxPublisher:                    outboxPublisherUseCase,
		Health:                             healthUseCase,
		Decision:                           decisionUseCase,
		CheckoutSession:                    checkoutSessionUseCase,
		OTP:                                otpUseCase,
		InstallmentLifecycle:               installmentLifecycleUseCase,
		InstallmentRepayment:               installmentRepaymentUseCase,
		FinancialCorePaymentResult:         financialCorePaymentResultUseCase,
		FinancialCorePaymentSession:        financialCorePaymentSessionUseCase,
		FinancialCorePaymentReconciliation: financialCorePaymentReconciliationUseCase,
		PaymentAttemptReconciliation:       paymentAttemptReconciliationUseCase,
		SagaExecutor:                       sagaExecutorUseCase,
		Contract:                           contractUseCase,
		Merchant:                           merchantUseCase,
		MerchantTerminal:                   merchantTerminalUseCase,
		CustomerAccount:                    customerAccountUseCase,
		CustomerCredit:                     customerCreditUseCase,
		ShortLink:                          shortLinkUseCase,
		RepaymentReminder:                  repaymentReminderUseCase,
		Collections:                        collectionsUseCase,
	}, nil
}

func nonEmptyHosts(hosts []string) []string {
	out := make([]string, 0, len(hosts))
	for _, host := range hosts {
		if trimmed := strings.TrimSpace(host); trimmed != "" {
			out = append(out, trimmed)
		}
	}

	return out
}
