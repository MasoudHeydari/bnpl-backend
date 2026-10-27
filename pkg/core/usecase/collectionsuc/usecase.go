package collectionsuc

import (
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/clock"
	"github.com/parstis/bnpl-backend/pkg/core/identifier"
	coremetrics "github.com/parstis/bnpl-backend/pkg/core/metrics"
	collectionsv1 "github.com/parstis/bnpl-backend/pkg/core/policy/collections/v1"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// UseCase assesses collections default entry and recovery for customers.
type UseCase struct {
	pool                repo.Pool
	customerProfile     repo.CustomerProfile
	installment         repo.Installment
	decision            repo.Decision
	defaultEpisode      repo.DefaultEpisode
	customerRestriction repo.CustomerRestriction
	collectionsCase     repo.CollectionsCase
	outbox              repo.Outbox
	clock               clock.Clock
	idGenerator         identifier.Generator
	businessMetrics     coremetrics.BusinessMetrics
	policy              collectionsv1.Policy
	auditDestination    string
	batchSize           int
}

// New constructs a collections assessment use case.
func New(
	pool repo.Pool,
	customerProfile repo.CustomerProfile,
	installment repo.Installment,
	decision repo.Decision,
	defaultEpisode repo.DefaultEpisode,
	customerRestriction repo.CustomerRestriction,
	collectionsCase repo.CollectionsCase,
	outbox repo.Outbox,
	clk clock.Clock,
	idGenerator identifier.Generator,
	businessMetrics coremetrics.BusinessMetrics,
	auditDestination string,
	batchSize int,
) (*UseCase, error) {
	const op = "collectionsuc.New"

	if pool == nil {
		return nil, fmt.Errorf("%s: pool is required", op)
	}

	if customerProfile == nil {
		return nil, fmt.Errorf("%s: customer profile repository is required", op)
	}

	if installment == nil {
		return nil, fmt.Errorf("%s: installment repository is required", op)
	}

	if decision == nil {
		return nil, fmt.Errorf("%s: decision repository is required", op)
	}

	if defaultEpisode == nil {
		return nil, fmt.Errorf("%s: default episode repository is required", op)
	}

	if customerRestriction == nil {
		return nil, fmt.Errorf("%s: customer restriction repository is required", op)
	}

	if collectionsCase == nil {
		return nil, fmt.Errorf("%s: collections case repository is required", op)
	}

	if outbox == nil {
		return nil, fmt.Errorf("%s: outbox repository is required", op)
	}

	if clk == nil {
		return nil, fmt.Errorf("%s: clock is required", op)
	}

	if idGenerator == nil {
		return nil, fmt.Errorf("%s: id generator is required", op)
	}

	if businessMetrics == nil {
		return nil, fmt.Errorf("%s: business metrics is required", op)
	}

	if auditDestination == "" {
		return nil, fmt.Errorf("%s: audit destination is required", op)
	}

	if batchSize <= 0 {
		return nil, fmt.Errorf("%s: batch size must be positive", op)
	}

	return &UseCase{
		pool:                pool,
		customerProfile:     customerProfile,
		installment:         installment,
		decision:            decision,
		defaultEpisode:      defaultEpisode,
		customerRestriction: customerRestriction,
		collectionsCase:     collectionsCase,
		outbox:              outbox,
		clock:               clk,
		idGenerator:         idGenerator,
		businessMetrics:     businessMetrics,
		policy:              collectionsv1.New(),
		auditDestination:    auditDestination,
		batchSize:           batchSize,
	}, nil
}
