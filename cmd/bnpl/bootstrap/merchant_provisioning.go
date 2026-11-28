package bootstrap

import (
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/merchantparticipationrp"
	merchantadapter "github.com/parstis/bnpl-backend/pkg/adapter/external/merchant"
	merchantprovisioningv1 "github.com/parstis/bnpl-backend/pkg/core/policy/merchantprovisioning/v1"
	coretask "github.com/parstis/bnpl-backend/pkg/core/task"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/merchantprovisioninguc"
)

func (a *App) newMerchantProvisioning() (*merchantprovisioninguc.UseCase, error) {
	const op = "bootstrap.App.newMerchantProvisioning"

	cfg := a.Config.MerchantProvisioning
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
		merchantprovisioningv1.New(),
	)
}

func (a *App) registerMerchantProvisioningTask() error {
	const op = "bootstrap.App.registerMerchantProvisioningTask"

	// The pilot cannot run financial or other business jobs. This job is always
	// enabled whenever the participation consumer is enabled.
	err := a.Infrastructure.Task.Register(coretask.Task{
		Name:    "merchant_progress_delivery",
		Enabled: true,
		Handler: "merchant_progress_delivery",
		Trigger: coretask.Trigger{
			Type:       coretask.TriggerInterval,
			Interval:   2 * time.Second,
			RunOnStart: true,
		},
		Job: a.MerchantProvisioning.DeliverDue,
	})
	if err != nil {
		return fmt.Errorf("%s: register delivery task: %w", op, err)
	}

	return nil
}
