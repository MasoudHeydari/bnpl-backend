package customercredituc

import (
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/policy/collections"

	"github.com/parstis/bnpl-backend/pkg/core/clock"
	extcreditassessment "github.com/parstis/bnpl-backend/pkg/core/external/creditassessment"
	"github.com/parstis/bnpl-backend/pkg/core/fingerprint"
	"github.com/parstis/bnpl-backend/pkg/core/identifier"
	"github.com/parstis/bnpl-backend/pkg/core/policy"
	"github.com/parstis/bnpl-backend/pkg/core/policy/initialcreditlimit"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// UseCase orchestrates customer credit summary reads and governed limit assignments.
type UseCase struct {
	collectionsPolicy      collections.Policy
	pool                   repo.Pool
	customerProfile        repo.CustomerProfile
	creditAccount          repo.CreditAccount
	decision               repo.Decision
	customerRestriction    repo.CustomerRestriction
	installment            repo.Installment
	defaultEpisode         repo.DefaultEpisode
	outbox                 repo.Outbox
	clock                  clock.Clock
	fingerprintHasher      fingerprint.Hasher
	creditAssessmentClient extcreditassessment.Client
	policyAssignment       repo.PolicyAssignment
	initialLimitPolicies   *initialcreditlimit.Registry
	defaultInitialLimitRef policy.Ref
	auditDestination       string
	idGenerator            identifier.Generator
}

// New constructs a customer credit use case.
func New(
	pool repo.Pool,
	customerProfile repo.CustomerProfile,
	creditAccount repo.CreditAccount,
	decision repo.Decision,
	customerRestriction repo.CustomerRestriction,
	installment repo.Installment,
	defaultEpisode repo.DefaultEpisode,
	outbox repo.Outbox,
	clk clock.Clock,
	fingerprintHasher fingerprint.Hasher,
	creditAssessmentClient extcreditassessment.Client,
	policyAssignment repo.PolicyAssignment,
	initialLimitPolicies *initialcreditlimit.Registry,
	defaultInitialLimitRef policy.Ref,
	auditDestination string,
	idGenerator identifier.Generator,
	collectionsPolicy collections.Policy,
) (*UseCase, error) {
	const op = "customercredituc.New"

	if collectionsPolicy == nil {
		return nil, fmt.Errorf("%s: collections policy is required", op)
	}

	if pool == nil {
		return nil, fmt.Errorf("%s: pool is required", op)
	}

	if customerProfile == nil {
		return nil, fmt.Errorf("%s: customer profile repository is required", op)
	}

	if creditAccount == nil {
		return nil, fmt.Errorf("%s: credit account repository is required", op)
	}

	if decision == nil {
		return nil, fmt.Errorf("%s: decision repository is required", op)
	}

	if customerRestriction == nil {
		return nil, fmt.Errorf("%s: customer restriction repository is required", op)
	}

	if installment == nil {
		return nil, fmt.Errorf("%s: installment repository is required", op)
	}

	if defaultEpisode == nil {
		return nil, fmt.Errorf("%s: default episode repository is required", op)
	}

	if outbox == nil {
		return nil, fmt.Errorf("%s: outbox repository is required", op)
	}

	if clk == nil {
		return nil, fmt.Errorf("%s: clock is required", op)
	}

	if fingerprintHasher == nil {
		return nil, fmt.Errorf("%s: fingerprint generator is required", op)
	}

	if creditAssessmentClient == nil {
		return nil, fmt.Errorf("%s: credit assessment client is required", op)
	}

	if policyAssignment == nil {
		return nil, fmt.Errorf("%s: policy assignment repository is required", op)
	}

	if initialLimitPolicies == nil {
		return nil, fmt.Errorf("%s: initial limit policy registry is required", op)
	}

	if defaultInitialLimitRef.ID == "" || defaultInitialLimitRef.Version == "" {
		return nil, fmt.Errorf("%s: default initial limit policy ref is required", op)
	}

	if auditDestination == "" {
		return nil, fmt.Errorf("%s: audit destination is required", op)
	}

	if idGenerator == nil {
		return nil, fmt.Errorf("%s: identifier generator is required", op)
	}

	return &UseCase{
		pool:                   pool,
		collectionsPolicy:      collectionsPolicy,
		customerProfile:        customerProfile,
		creditAccount:          creditAccount,
		decision:               decision,
		customerRestriction:    customerRestriction,
		installment:            installment,
		defaultEpisode:         defaultEpisode,
		outbox:                 outbox,
		clock:                  clk,
		fingerprintHasher:      fingerprintHasher,
		creditAssessmentClient: creditAssessmentClient,
		policyAssignment:       policyAssignment,
		initialLimitPolicies:   initialLimitPolicies,
		defaultInitialLimitRef: defaultInitialLimitRef,
		auditDestination:       auditDestination,
		idGenerator:            idGenerator,
	}, nil
}
