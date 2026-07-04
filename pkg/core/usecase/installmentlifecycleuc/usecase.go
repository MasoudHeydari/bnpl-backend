package installmentlifecycleuc

import (
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/clock"
	"github.com/parstis/bnpl-backend/pkg/core/identifier"
	coremetrics "github.com/parstis/bnpl-backend/pkg/core/metrics"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// UseCase orchestrates installment lifecycle batch maintenance.
type UseCase struct {
	pool              repo.Pool
	installment       repo.Installment
	installmentCharge repo.InstallmentCharge
	outbox            repo.Outbox
	clock             clock.Clock
	auditDestination  string
	idGenerator       identifier.Generator
	businessMetrics   coremetrics.BusinessMetrics
	// MarkDueBatchSize is the maximum scheduled-to-due transitions per run.
	MarkDueBatchSize int
	// MarkOverdueBatchSize is the maximum due-to-overdue transitions per run.
	MarkOverdueBatchSize int
}

// New constructs an installment lifecycle use case.
func New(
	pool repo.Pool,
	installment repo.Installment,
	installmentCharge repo.InstallmentCharge,
	outbox repo.Outbox,
	clk clock.Clock,
	auditDestination string,
	idGenerator identifier.Generator,
	markDueBatchSize int,
	markOverdueBatchSize int,
	businessMetrics coremetrics.BusinessMetrics,
) (*UseCase, error) {
	const op = "installmentlifecycleuc.New"

	if auditDestination == "" {
		return nil, fmt.Errorf("%s: installment lifecycle audit destination is required", op)
	}

	if idGenerator == nil {
		return nil, fmt.Errorf("%s: installment lifecycle id generator is required", op)
	}

	if markDueBatchSize <= 0 {
		return nil, fmt.Errorf("%s: installment mark due batch size must be positive", op)
	}

	if markOverdueBatchSize <= 0 {
		return nil, fmt.Errorf("%s: installment mark overdue batch size must be positive", op)
	}

	if businessMetrics == nil {
		return nil, fmt.Errorf("%s: business metrics is required", op)
	}

	return &UseCase{
		pool:                 pool,
		installment:          installment,
		installmentCharge:    installmentCharge,
		outbox:               outbox,
		clock:                clk,
		auditDestination:     auditDestination,
		idGenerator:          idGenerator,
		businessMetrics:      businessMetrics,
		MarkDueBatchSize:     markDueBatchSize,
		MarkOverdueBatchSize: markOverdueBatchSize,
	}, nil
}
