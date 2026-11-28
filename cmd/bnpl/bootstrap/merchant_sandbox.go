package bootstrap

import (
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/merchantparticipationrp"
	merchantadapter "github.com/parstis/bnpl-backend/pkg/adapter/external/merchant"
	coretask "github.com/parstis/bnpl-backend/pkg/core/task"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/merchantprovisioninguc"
)

func (a *App) newMerchantSandbox() (*merchantprovisioninguc.UseCase, error) {
	const op = "bootstrap.App.newMerchantSandbox"
	cfg := a.Config.MerchantSandbox
	client, err := merchantadapter.New(a.Infrastructure.HTTPClient, a.Infrastructure.CircuitBreaker, cfg.BaseURL, cfg.Token)
	if err != nil {
		return nil, fmt.Errorf("%s: create Merchant client: %w", op, err)
	}

	return merchantprovisioninguc.New(
		a.Pool,
		a.Repos.Inbox,
		merchantparticipationrp.New(),
		a.Repos.MerchantProfile,
		client,
		a.Infrastructure.Clock,
		a.Infrastructure.DomainUUIDGenerator,
		a.Infrastructure.Logger,
	)
}

func (a *App) registerMerchantSandboxTask() error {
	const op = "bootstrap.App.registerMerchantSandboxTask"
	// The pilot cannot run financial or other business jobs. This job is always
	// enabled whenever the participation consumer is enabled.
	err := a.Infrastructure.Task.Register(coretask.Task{
		Name: "merchant_progress_delivery", Enabled: true, Handler: "merchant_progress_delivery",
		Trigger: coretask.Trigger{Type: coretask.TriggerInterval, Interval: 2 * time.Second, RunOnStart: true},
		Job:     a.MerchantSandbox.DeliverDue,
	})
	if err != nil {
		return fmt.Errorf("%s: register delivery task: %w", op, err)
	}

	return nil
}
