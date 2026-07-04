package config

import (
	"fmt"
	"io"
	"time"

	adapterbotmessage "github.com/parstis/bnpl-backend/pkg/adapter/external/botmessage"
	botmessagebale "github.com/parstis/bnpl-backend/pkg/adapter/external/botmessage/bale"
	botmessagenoop "github.com/parstis/bnpl-backend/pkg/adapter/external/botmessage/noop"
	botmessagetelegram "github.com/parstis/bnpl-backend/pkg/adapter/external/botmessage/telegram"
	checkoutpaymentext "github.com/parstis/bnpl-backend/pkg/adapter/external/checkoutpayment"
	checkoutpaymentgrpc "github.com/parstis/bnpl-backend/pkg/adapter/external/checkoutpayment/grpc"
	checkoutpaymentnoop "github.com/parstis/bnpl-backend/pkg/adapter/external/checkoutpayment/noop"
	contractcancellationnoop "github.com/parstis/bnpl-backend/pkg/adapter/external/contractcancellation/noop"
	creditassessmentext "github.com/parstis/bnpl-backend/pkg/adapter/external/creditassessment"
	"github.com/parstis/bnpl-backend/pkg/adapter/external/creditassessment/creditbureau"
	creditassessmentnoop "github.com/parstis/bnpl-backend/pkg/adapter/external/creditassessment/noop"
	installmentpaymentnoop "github.com/parstis/bnpl-backend/pkg/adapter/external/installmentpayment/noop"
	"github.com/parstis/bnpl-backend/pkg/core/circuitbreaker"
	coreclock "github.com/parstis/bnpl-backend/pkg/core/clock"
	extcheckoutpayment "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
	extcontractcancellation "github.com/parstis/bnpl-backend/pkg/core/external/contractcancellation"
	extcreditassessment "github.com/parstis/bnpl-backend/pkg/core/external/creditassessment"
	extinstallmentpayment "github.com/parstis/bnpl-backend/pkg/core/external/installmentpayment"
	corehttpclient "github.com/parstis/bnpl-backend/pkg/core/httpclient"
	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// External groups configuration for business-facing external systems.
type External struct {
	// CreditAssessment configures the external credit assessment integration.
	CreditAssessment ExternalCreditAssessment `yaml:"credit_assessment"`

	// FinancialCore configures Financial Core external integrations.
	FinancialCore ExternalFinancialCore `yaml:"financial_core"`

	// Customer configures the Customer Reference Service integration used by
	// Partner checkout for mobile → external customer_id resolution.
	Customer ExternalCustomer `yaml:"customer"`

	// Notification configures the Notification Service integration used by
	// hosted external checkout OTP submission.
	Notification ExternalNotification `yaml:"notification"`

	// BotMessaging configures outbound and inbound bot messaging integrations.
	BotMessaging ExternalBotMessaging `yaml:"bot_messaging"`

	// WebhookDispatcher configures outbound merchant webhook delivery.
	WebhookDispatcher ExternalWebhookDispatcher `yaml:"webhook_dispatcher"`
}

// ExternalBotMessaging configures bot-based messaging integrations.
//
// Noop is mutually exclusive with real providers. Telegram and Bale may both
// be enabled at the same time; the resulting manager listens to both and routes
// outbound messages by Destination.IntegrationID.
type ExternalBotMessaging struct {
	// Noop configures the disabled/local bot messaging provider.
	Noop ExternalBotMessagingNoop `yaml:"noop"`

	// Telegram configures Telegram Bot API integration.
	Telegram ExternalBotMessagingTelegram `yaml:"telegram"`

	// Bale configures Bale Bot API integration.
	Bale ExternalBotMessagingBale `yaml:"bale"`
}

// ExternalBotMessagingNoop holds noop provider settings.
type ExternalBotMessagingNoop struct {
	// Enabled selects noop as the only bot messaging integration.
	Enabled bool `yaml:"enabled"`
}

// ExternalBotMessagingTelegram configures Telegram Bot API integration.
type ExternalBotMessagingTelegram struct {
	// Enabled starts Telegram outbound routing and inbound long polling.
	Enabled bool `yaml:"enabled"`

	// Token is the BotFather token. Secret prevents accidental config/log output.
	Token model.Secret `yaml:"token"`

	// PollTimeout controls the server-side getUpdates long-poll duration.
	PollTimeout time.Duration `yaml:"poll_timeout"`

	// RetryBaseDelay is the first backoff after transient polling failures.
	RetryBaseDelay time.Duration `yaml:"retry_base_delay"`

	// RetryMaxDelay caps transient polling backoff.
	RetryMaxDelay time.Duration `yaml:"retry_max_delay"`
}

// ExternalBotMessagingBale configures Bale Bot API integration.
type ExternalBotMessagingBale struct {
	// Enabled starts Bale outbound routing and inbound long polling.
	Enabled bool `yaml:"enabled"`

	// Token is the Bale bot token. Secret prevents accidental config/log output.
	Token model.Secret `yaml:"token"`

	// PollTimeout controls the server-side getUpdates long-poll duration.
	PollTimeout time.Duration `yaml:"poll_timeout"`

	// RetryBaseDelay is the first backoff after transient polling failures.
	RetryBaseDelay time.Duration `yaml:"retry_base_delay"`

	// RetryMaxDelay caps transient polling backoff.
	RetryMaxDelay time.Duration `yaml:"retry_max_delay"`
}

func (e ExternalBotMessaging) newManager(
	httpClient corehttpclient.Client,
	logger corelog.Logger,
) (*adapterbotmessage.Manager, error) {
	const op = "config.ExternalBotMessaging.newManager"

	integrations := make([]adapterbotmessage.Integration, 0, 2)

	if e.Noop.Enabled {
		integrations = append(integrations, botmessagenoop.New())
	}

	if e.Telegram.Enabled {
		client, err := botmessagetelegram.New(
			botmessagetelegram.Config{
				Token:          e.Telegram.Token.Reveal(),
				PollTimeout:    e.Telegram.PollTimeout,
				RetryBaseDelay: e.Telegram.RetryBaseDelay,
				RetryMaxDelay:  e.Telegram.RetryMaxDelay,
			},
			httpClient,
			logger,
		)
		if err != nil {
			return nil, fmt.Errorf("%s: initialize telegram bot messaging adapter: %w", op, err)
		}

		integrations = append(integrations, client)
	}

	if e.Bale.Enabled {
		client, err := botmessagebale.New(
			botmessagebale.Config{
				Token:          e.Bale.Token.Reveal(),
				PollTimeout:    e.Bale.PollTimeout,
				RetryBaseDelay: e.Bale.RetryBaseDelay,
				RetryMaxDelay:  e.Bale.RetryMaxDelay,
			},
			httpClient,
			logger,
		)
		if err != nil {
			return nil, fmt.Errorf("%s: initialize bale bot messaging adapter: %w", op, err)
		}

		integrations = append(integrations, client)
	}

	// Missing bot_messaging config is kept backward-compatible as noop. The
	// shipped config is explicit, but older deployment files do not need a
	// schema-breaking migration merely to disable an optional integration.
	if len(integrations) == 0 {
		integrations = append(integrations, botmessagenoop.New())
	}

	return adapterbotmessage.NewManager(integrations...)
}

// ExternalCreditAssessment configures the external credit assessment signal integration.
type ExternalCreditAssessment struct {
	// Provider selects the credit assessment client implementation.
	Provider string `yaml:"provider"`

	// Noop contains configuration for the noop credit assessment provider.
	Noop ExternalCreditAssessmentNoop `yaml:"noop"`

	// Static contains configuration for the static credit assessment provider.
	Static ExternalCreditAssessmentStatic `yaml:"static"`

	// CreditBureau contains configuration for the credit bureau provider.
	CreditBureau ExternalCreditAssessmentCreditBureau `yaml:"creditbureau"`
}

// ExternalCreditAssessmentNoop holds noop provider settings.
type ExternalCreditAssessmentNoop struct{}

// ExternalCreditAssessmentStatic holds static provider settings.
type ExternalCreditAssessmentStatic struct{}

// ExternalCreditAssessmentCreditBureau holds credit bureau provider settings.
type ExternalCreditAssessmentCreditBureau struct{}

func (e ExternalCreditAssessment) newExternalCreditAssessmentClient(
	breaker circuitbreaker.CircuitBreaker,
	clk coreclock.Clock,
) (extcreditassessment.Client, error) {
	const op = "config.ExternalCreditAssessment.newExternalCreditAssessmentClient"

	provider := creditassessmentext.Provider(e.Provider)

	switch provider {
	case creditassessmentext.NoopProvider:
		return creditassessmentnoop.New(breaker, clk)

	case creditassessmentext.CreditBureauProvider:
		return creditbureau.New(breaker)

	default:
		return nil, fmt.Errorf("%s: unsupported external credit assessment provider: %q", op, e.Provider)
	}
}

// ExternalFinancialCore configures Financial Core integrations.
type ExternalFinancialCore struct {
	// Provider selects the Financial Core provider implementation.
	Provider string `yaml:"provider"`

	// PaymentReturnURL is the configured HTTPS redirect URL sent to Financial Core
	// after IPG checkout. API clients do not supply this value.
	PaymentReturnURL string `yaml:"payment_return_url"`

	// Noop contains configuration for the noop Financial Core provider.
	Noop ExternalFinancialCoreNoop `yaml:"noop"`

	// GRPC configures the Financial Core gRPC client.
	GRPC ExternalFinancialCoreGRPC `yaml:"grpc"`
}

// ExternalFinancialCoreNoop holds noop provider settings.
type ExternalFinancialCoreNoop struct{}

// ExternalFinancialCoreGRPC configures the Financial Core gRPC client.
type ExternalFinancialCoreGRPC struct {
	// Address is the Financial Core gRPC server address.
	Address string `yaml:"address"`

	// Timeout configures the per-RPC timeout for Financial Core gRPC calls.
	Timeout time.Duration `yaml:"timeout"`

	// Insecure controls whether TLS is disabled for the Financial Core gRPC connection.
	Insecure bool `yaml:"insecure"`
}

func (e ExternalFinancialCore) newExternalCheckoutPaymentClient(
	breaker circuitbreaker.CircuitBreaker,
) (extcheckoutpayment.Client, io.Closer, error) {
	const op = "config.ExternalFinancialCore.newExternalCheckoutPaymentClient"

	provider := checkoutpaymentext.Provider(e.Provider)

	switch provider {
	case checkoutpaymentext.GRPCProvider:
		return e.newGRPCExternalCheckoutPaymentClient(breaker)

	case checkoutpaymentext.NoopProvider:
		return checkoutpaymentnoop.New(), nil, nil

	default:
		return nil, nil, fmt.Errorf("%s: unsupported external financial core provider: %q", op, e.Provider)
	}
}

func (e ExternalFinancialCore) newGRPCExternalCheckoutPaymentClient(
	breaker circuitbreaker.CircuitBreaker,
) (extcheckoutpayment.Client, io.Closer, error) {
	const op = "config.ExternalFinancialCore.newGRPCExternalCheckoutPaymentClient"

	if e.GRPC.Address == "" {
		return nil, nil, fmt.Errorf("%s: invalid external.financial_core.grpc.address: empty", op)
	}

	conn, err := checkoutpaymentgrpc.Dial(e.GRPC.Address, e.GRPC.Insecure)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: initialize external financial core grpc client: %w", op, err)
	}

	return checkoutpaymentgrpc.New(conn, e.GRPC.Timeout, breaker), conn, nil
}

func (e ExternalFinancialCore) newExternalInstallmentPaymentClient(
	_ circuitbreaker.CircuitBreaker,
	clk coreclock.Clock,
) (extinstallmentpayment.Client, error) {
	const op = "config.ExternalFinancialCore.newExternalInstallmentPaymentClient"

	provider := checkoutpaymentext.Provider(e.Provider)

	switch provider {
	case checkoutpaymentext.NoopProvider, checkoutpaymentext.GRPCProvider:
		// Installment repayment is event-driven today; the direct client is only
		// used in local development until a gRPC installment payment adapter exists.
		return installmentpaymentnoop.New(clk)

	default:
		return nil, fmt.Errorf("%s: unsupported external financial core provider: %q", op, e.Provider)
	}
}

func (e ExternalFinancialCore) newExternalContractCancellationClient() (
	extcontractcancellation.Client,
	error,
) {
	const op = "config.ExternalFinancialCore.newExternalContractCancellationClient"

	provider := checkoutpaymentext.Provider(e.Provider)

	switch provider {
	case checkoutpaymentext.NoopProvider, checkoutpaymentext.GRPCProvider:
		// Real Financial Core cancellation RPC is not available yet. The noop
		// adapter refuses to fabricate financial success; production already
		// rejects financial_core.provider=noop.
		return contractcancellationnoop.New(), nil

	default:
		return nil, fmt.Errorf("%s: unsupported external financial core provider: %q", op, e.Provider)
	}
}
