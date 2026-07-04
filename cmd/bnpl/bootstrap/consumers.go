package bootstrap

import (
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/messaging/consumers"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/messaging/consumers/paymentreconciliation"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/messaging/consumers/paymentresult"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/messaging/consumers/paymentsession"
)

// RegisterConsumers registers startup-known Kafka message handlers.
//
// Topic → handler bindings are fixed at bootstrap from configuration. Dynamic
// or per-request consumer registration does not belong here.
func (a *App) RegisterConsumers() (*consumers.Registry, error) {
	const op = "bootstrap.App.RegisterConsumers"

	registry, err := consumers.NewRegistry(a.Infrastructure.Logger, a.Infrastructure.ErrorReporter)
	if err != nil {
		return nil, fmt.Errorf("%s: create consumer registry: %w", op, err)
	}

	paymentResultHandler := paymentresult.New(
		a.UseCases.FinancialCorePaymentResult,
		a.Infrastructure.Logger,
	)

	if err := registry.Register(
		a.Config.PaymentResultsTopic(),
		paymentResultHandler.Handle,
	); err != nil {
		return nil, fmt.Errorf("%s: register payment result consumer: %w", op, err)
	}

	paymentSessionHandler := paymentsession.New(
		a.UseCases.FinancialCorePaymentSession,
		a.Infrastructure.Logger,
	)

	if err := registry.Register(
		a.Config.InstallmentPaymentSessionsTopic(),
		paymentSessionHandler.Handle,
	); err != nil {
		return nil, fmt.Errorf("%s: register payment session consumer: %w", op, err)
	}

	paymentReconciliationHandler := paymentreconciliation.New(
		a.UseCases.FinancialCorePaymentReconciliation,
		a.Infrastructure.Logger,
	)

	if err := registry.Register(
		a.Config.InstallmentPaymentReconciliationResultsTopic(),
		paymentReconciliationHandler.Handle,
	); err != nil {
		return nil, fmt.Errorf("%s: register payment reconciliation consumer: %w", op, err)
	}

	return registry, nil
}
