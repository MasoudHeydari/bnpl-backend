package installmentlifecycleuc

import "context"

// Sweep names reported to Observer.SweepCompleted.
const (
	// SweepMarkDue is the scheduled-to-due transition sweep.
	SweepMarkDue = "mark_due"

	// SweepMarkOverdue is the due-to-overdue transition sweep.
	SweepMarkOverdue = "mark_overdue"

	// SweepAssessLateFees is the overdue late-fee assessment sweep.
	SweepAssessLateFees = "assess_late_fees"

	// SweepExpireOffered is the unattached offered installment expiry sweep.
	SweepExpireOffered = "expire_offered"
)

// Observer records operational flow checkpoints for installment lifecycle
// batch maintenance. Implementations must not modify domain state or return
// errors that affect business execution.
type Observer interface {
	// SweepCompleted records a committed sweep that processed at least one
	// installment. Empty sweeps are not reported.
	SweepCompleted(ctx context.Context, sweep string, processedCount int64)
}
