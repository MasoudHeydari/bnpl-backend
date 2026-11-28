package merchantprovisioninguc

import (
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/clock"
	"github.com/parstis/bnpl-backend/pkg/core/external/merchant"
	"github.com/parstis/bnpl-backend/pkg/core/identifier"
	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/policy/merchantprovisioning"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// UseCase coordinates provisioning and durable result delivery.
type UseCase struct {
	pool            repo.Pool
	inbox           repo.Inbox
	participation   repo.MerchantParticipation
	merchantProfile repo.MerchantProfile
	client          merchant.Client
	clock           clock.Clock
	ids             identifier.Generator
	logger          log.Logger
	policy          merchantprovisioning.Policy
}

// New requires all collaborators at construction.
func New(
	pool repo.Pool,
	inbox repo.Inbox,
	participation repo.MerchantParticipation,
	merchantProfile repo.MerchantProfile,
	client merchant.Client,
	clk clock.Clock,
	ids identifier.Generator,
	logger log.Logger,
	policy merchantprovisioning.Policy,
) (*UseCase, error) {
	const op = "merchantprovisioninguc.New"

	if pool == nil ||
		inbox == nil ||
		participation == nil ||
		merchantProfile == nil ||
		client == nil ||
		clk == nil ||
		ids == nil ||
		logger == nil ||
		policy == nil {
		return nil, fmt.Errorf("%s: all dependencies are required", op)
	}

	return &UseCase{
		pool:            pool,
		inbox:           inbox,
		participation:   participation,
		merchantProfile: merchantProfile,
		client:          client,
		clock:           clk,
		ids:             ids,
		logger:          logger,
		policy:          policy,
	}, nil
}
