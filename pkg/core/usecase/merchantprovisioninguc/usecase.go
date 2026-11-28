package merchantprovisioninguc

import (
	"errors"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/clock"
	"github.com/parstis/bnpl-backend/pkg/core/external/merchant"
	"github.com/parstis/bnpl-backend/pkg/core/identifier"
	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// ErrConflict marks an immutable event or command identity conflict.
var ErrConflict = errors.New("merchant participation identity conflict")

// ErrInvalidReference marks a conclusive invalid participation reference.
var ErrInvalidReference = errors.New("invalid merchant participation reference")

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
) (*UseCase, error) {
	const op = "merchantprovisioninguc.New"

	if pool == nil || inbox == nil || participation == nil || merchantProfile == nil || client == nil || clk == nil || ids == nil || logger == nil {
		return nil, fmt.Errorf("%s: all dependencies are required", op)
	}

	return &UseCase{pool: pool, inbox: inbox, participation: participation, merchantProfile: merchantProfile, client: client, clock: clk, ids: ids, logger: logger}, nil
}

// Input is the normalized command; the original envelope is retained in the inbox.
type Input struct {
	Event         model.InboxEvent
	MerchantID    model.ExternalMerchantID
	ApplicationID model.MerchantApplicationID
	RequestID     model.MerchantParticipationRequestID
	Environment   string
}
