package config

import (
	"github.com/parstis/bnpl-backend/pkg/core/clock"
	"github.com/parstis/bnpl-backend/pkg/core/external/merchant"
	"github.com/parstis/bnpl-backend/pkg/core/identifier"
	"github.com/parstis/bnpl-backend/pkg/core/log"
	merchantprovisioningv1 "github.com/parstis/bnpl-backend/pkg/core/policy/merchantprovisioning/v1"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/merchantprovisioninguc"
)

// NewMerchantProvisioningUseCase assembles the configured participation policy and dependencies.
func (c *Config) NewMerchantProvisioningUseCase(
	pool repo.Pool,
	inbox repo.Inbox,
	participation repo.MerchantParticipation,
	merchantProfile repo.MerchantProfile,
	client merchant.Client,
	clk clock.Clock,
	ids identifier.Generator,
	logger log.Logger,
) (*merchantprovisioninguc.UseCase, error) {
	return merchantprovisioninguc.New(
		pool,
		inbox,
		participation,
		merchantProfile,
		client,
		clk,
		ids,
		logger,
		merchantprovisioningv1.New(),
	)
}
