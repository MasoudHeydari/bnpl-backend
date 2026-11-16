package observer

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/event"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/field"
	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/installmentlifecycleuc"
)

// InstallmentLifecycle records installment lifecycle sweep checkpoints.
type InstallmentLifecycle struct {
	logger log.Logger
}

// NewInstallmentLifecycle constructs an installment lifecycle observer.
func NewInstallmentLifecycle(logger log.Logger) (*InstallmentLifecycle, error) {
	const op = "observer.NewInstallmentLifecycle"

	if logger == nil {
		return nil, fmt.Errorf("%s: logger is required", op)
	}

	return &InstallmentLifecycle{logger: logger}, nil
}

var _ installmentlifecycleuc.Observer = (*InstallmentLifecycle)(nil)

// SweepCompleted implements the corresponding Observer checkpoint.
func (o *InstallmentLifecycle) SweepCompleted(ctx context.Context, sweep string, processedCount int64) {
	o.logger.WithContext(ctx).Info(
		event.InstallmentLifecycleSweepCompleted,
		field.JobName(sweep),
		field.ProcessedCount(int(processedCount)),
	)
}
