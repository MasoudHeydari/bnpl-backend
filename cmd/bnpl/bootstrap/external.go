package bootstrap

import (
	"errors"
	"fmt"
	"io"

	adapterbotmessage "github.com/parstis/bnpl-backend/pkg/adapter/external/botmessage"
	checkoutgrpc "github.com/parstis/bnpl-backend/pkg/adapter/external/checkoutpayment/grpc"
	collectiongrpc "github.com/parstis/bnpl-backend/pkg/adapter/external/collectionpayment/grpc"
	collectionnoop "github.com/parstis/bnpl-backend/pkg/adapter/external/collectionpayment/noop"
	extcheckoutpayment "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
	extcollection "github.com/parstis/bnpl-backend/pkg/core/external/collectionpayment"
	extcontractcancellation "github.com/parstis/bnpl-backend/pkg/core/external/contractcancellation"
	"github.com/parstis/bnpl-backend/pkg/core/external/creditassessment"
	extcustomer "github.com/parstis/bnpl-backend/pkg/core/external/customer"
	extinstallmentpayment "github.com/parstis/bnpl-backend/pkg/core/external/installmentpayment"
	extmerchant "github.com/parstis/bnpl-backend/pkg/core/external/merchant"
	extnotification "github.com/parstis/bnpl-backend/pkg/core/external/notification"
)

// External holds business-facing external system adapters assembled for App.
//
// These clients implement core external ports (credit assessment, Financial
// Core payment/cancellation). Transport details stay
// in adapter packages.
type External struct {
	// BotMessaging coordinates all enabled Telegram/Bale bot integrations.
	BotMessaging *adapterbotmessage.Manager

	// CreditAssessmentClient evaluates customer credit with the configured provider.
	CreditAssessmentClient creditassessment.Client

	// CheckoutPaymentClient creates checkout payment sessions with Financial Core.
	CheckoutPaymentClient extcheckoutpayment.Client

	// CollectionPaymentClient implements FC CollectionPayment v1 (target draft)
	// aggregate repayment RPCs. Its noop mode fails closed instead of fabricating
	// a paid result.
	CollectionPaymentClient extcollection.Client

	// CustomerResolver maps a merchant-asserted mobile to a Customer Reference
	// Service customer_id (stored as customer_profiles.external_customer_id).
	CustomerResolver extcustomer.Resolver

	// MerchantClient resolves participation and delivers progress reports.
	MerchantClient extmerchant.Client

	// NotificationSubmitter durably submits checkout OTP notifications.
	NotificationSubmitter extnotification.Submitter

	// InstallmentPaymentClient initiates installment repayment payments.
	InstallmentPaymentClient extinstallmentpayment.Client

	// ContractCancellationClient requests contract cancellation from Financial Core.
	ContractCancellationClient extcontractcancellation.Client

	// closers tracks closable transports created during NewExternal.
	closers []io.Closer
}

// Close releases external system resources such as gRPC client connections.
func (e External) Close() error {
	var err error

	for _, closer := range e.closers {
		if closer == nil {
			continue
		}

		err = errors.Join(err, closer.Close())
	}

	return err
}

// NewExternal constructs external system adapters required by App.
//
// Providers are selected from configuration. Closable transports (for example
// gRPC connections) are tracked so Close can
// release them on partial failure or process shutdown.
func (a *App) NewExternal() (_ External, err error) {
	const op = "bootstrap.App.NewExternal"

	external := External{}

	// Close adapters already created when a later constructor fails. Without
	// this guard, a partially initialized application leaks gRPC connections.
	defer func() {
		if err != nil {
			err = errors.Join(err, external.Close())
		}
	}()

	external.BotMessaging, err = a.Config.NewExternalBotMessagingManager(
		a.Infrastructure.HTTPClient,
		a.Infrastructure.Logger,
	)
	if err != nil {
		return External{}, fmt.Errorf("%s: initialize external bot messaging: %w", op, err)
	}

	external.CreditAssessmentClient, err = a.Config.NewExternalCreditAssessmentClient(
		a.Infrastructure.CircuitBreaker,
		a.Infrastructure.Clock,
	)
	if err != nil {
		return External{}, fmt.Errorf("%s: initialize external credit assessment client: %w", op, err)
	}

	var checkoutPaymentCloser io.Closer
	external.CheckoutPaymentClient, checkoutPaymentCloser, err = a.Config.NewExternalCheckoutPaymentClient(
		a.Infrastructure.CircuitBreaker,
		a.Infrastructure.Logger,
	)
	if err != nil {
		return External{}, fmt.Errorf("%s: initialize external checkout payment client: %w", op, err)
	}

	if checkoutPaymentCloser != nil {
		external.closers = append(external.closers, checkoutPaymentCloser)
	}

	// CollectionPayment v1 (FC target draft) uses the same already-configured Financial Core
	// transport security as CheckoutPayment. Keep the client optional at the
	// provider level; the old Kafka-dependent flows remain intact until the
	// durable query-based replacement is fully cut over.
	switch a.Config.External.FinancialCore.Provider {
	case "noop":
		external.CollectionPaymentClient = collectionnoop.New()

	case "grpc":
		financialConfig := a.Config.External.FinancialCore.GRPC
		conn, dialErr := checkoutgrpc.Dial(
			financialConfig.Address,
			financialConfig.Insecure,
			checkoutgrpc.TLSConfig{
				CAFile:     financialConfig.TLS.CAFile,
				CertFile:   financialConfig.TLS.CertFile,
				KeyFile:    financialConfig.TLS.KeyFile,
				ServerName: financialConfig.TLS.ServerName,
			},
		)
		if dialErr != nil {
			return External{}, fmt.Errorf("%s: dial collection payment service: %w", op, dialErr)
		}

		external.closers = append(external.closers, conn)
		external.CollectionPaymentClient, err = collectiongrpc.New(
			conn,
			financialConfig.Timeout,
			a.Infrastructure.Logger,
		)
		if err != nil {
			return External{}, fmt.Errorf("%s: initialize collection payment v1 client: %w", op, err)
		}
	default:
		return External{}, fmt.Errorf("%s: unsupported Financial Core provider for collection payment v1: %q", op, a.Config.External.FinancialCore.Provider)
	}

	var customerCloser io.Closer
	external.CustomerResolver, customerCloser, err = a.Config.NewExternalCustomerClient(
		a.Infrastructure.CircuitBreaker,
		a.Infrastructure.Logger,
	)
	if err != nil {
		return External{}, fmt.Errorf("%s: initialize external customer client: %w", op, err)
	}

	if customerCloser != nil {
		external.closers = append(external.closers, customerCloser)
	}

	var merchantCloser io.Closer
	external.MerchantClient, merchantCloser, err = a.Config.NewExternalMerchantClient(
		a.Infrastructure.CircuitBreaker,
		a.Infrastructure.Logger,
	)
	if err != nil {
		return External{}, fmt.Errorf("%s: initialize external merchant client: %w", op, err)
	}

	if merchantCloser != nil {
		external.closers = append(external.closers, merchantCloser)
	}

	var notificationCloser io.Closer
	external.NotificationSubmitter, notificationCloser, err = a.Config.NewExternalNotificationClient(
		a.Infrastructure.CircuitBreaker,
		a.Infrastructure.Logger,
	)
	if err != nil {
		return External{}, fmt.Errorf("%s: initialize external notification client: %w", op, err)
	}

	if notificationCloser != nil {
		external.closers = append(external.closers, notificationCloser)
	}

	external.InstallmentPaymentClient, err = a.Config.NewExternalInstallmentPaymentClient(
		a.Infrastructure.CircuitBreaker,
		a.Infrastructure.Clock,
	)
	if err != nil {
		return External{}, fmt.Errorf("%s: initialize external installment payment client: %w", op, err)
	}

	external.ContractCancellationClient, err = a.Config.NewExternalContractCancellationClient()
	if err != nil {
		return External{}, fmt.Errorf("%s: initialize external contract cancellation client: %w", op, err)
	}

	return external, nil
}
