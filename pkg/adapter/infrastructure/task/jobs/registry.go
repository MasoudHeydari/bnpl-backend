package jobs

import (
	"fmt"

	checkoutsessionjob "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/task/jobs/checkoutsession"
	contractjob "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/task/jobs/contract"
	installmentjob "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/task/jobs/installment"
	merchantterminaljob "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/task/jobs/merchantterminal"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/task/jobs/outbox"
	paymentattemptjob "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/task/jobs/paymentattempt"
	repaymentreminderjob "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/task/jobs/repaymentreminder"
	runtimejob "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/task/jobs/runtime"
	sagajob "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/task/jobs/saga"
	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
	coretask "github.com/parstis/bnpl-backend/pkg/core/task"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/checkoutsessionuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/contractuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/installmentlifecycleuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/merchantterminaluc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/outboxpublisheruc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/paymentattemptreconciliationuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/repaymentreminderuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/sagaexecutoruc"
)

// Registry resolves task handlers to executable jobs.
type Registry struct {
	logger                         corelog.Logger
	outboxpublisheruc              *outboxpublisheruc.UseCase
	installmentlifecycleuc         *installmentlifecycleuc.UseCase
	contractuc                     *contractuc.UseCase
	checkoutsessionuc              *checkoutsessionuc.UseCase
	paymentattemptreconciliationuc *paymentattemptreconciliationuc.UseCase
	sagaexecutoruc                 *sagaexecutoruc.UseCase
	merchantterminaluc             *merchantterminaluc.UseCase
	repaymentreminderuc            *repaymentreminderuc.UseCase
}

// NewRegistry creates a task job registry.
func NewRegistry(
	logger corelog.Logger,
	outboxpublisheruc *outboxpublisheruc.UseCase,
	installmentlifecycleuc *installmentlifecycleuc.UseCase,
	contractuc *contractuc.UseCase,
	checkoutsessionuc *checkoutsessionuc.UseCase,
	paymentattemptreconciliationuc *paymentattemptreconciliationuc.UseCase,
	sagaexecutoruc *sagaexecutoruc.UseCase,
	merchantterminaluc *merchantterminaluc.UseCase,
	repaymentreminderuc *repaymentreminderuc.UseCase,
) Registry {
	return Registry{
		logger:                         logger,
		outboxpublisheruc:              outboxpublisheruc,
		installmentlifecycleuc:         installmentlifecycleuc,
		contractuc:                     contractuc,
		checkoutsessionuc:              checkoutsessionuc,
		paymentattemptreconciliationuc: paymentattemptreconciliationuc,
		sagaexecutoruc:                 sagaexecutoruc,
		merchantterminaluc:             merchantterminaluc,
		repaymentreminderuc:            repaymentreminderuc,
	}
}

// Resolve resolves a configured handler to an executable job.
func (r Registry) Resolve(handler string) (coretask.Job, error) {
	const op = "jobs.Registry.Resolve"

	switch handler {
	case "runtime_goroutine_metrics":
		return runtimejob.GoroutineMetrics(r.logger), nil

	case "outbox_publisher":
		return outbox.Publisher(r.outboxpublisheruc), nil

	case "installment_offer_expiry":
		return installmentjob.Expiry(r.logger, r.installmentlifecycleuc), nil

	case "contract_payment_pending_expiry":
		return contractjob.PaymentPendingExpiry(r.logger, r.contractuc), nil

	case "checkout_session_expiry":
		return checkoutsessionjob.Expiry(r.logger, r.checkoutsessionuc), nil

	case "checkout_session_confirming_recovery":
		return checkoutsessionjob.RecoverConfirming(r.logger, r.checkoutsessionuc), nil

	case "saga_executor":
		return sagajob.Executor(r.logger, r.sagaexecutoruc), nil

	case "installment_mark_due":
		return installmentjob.MarkDue(r.logger, r.installmentlifecycleuc), nil

	case "installment_mark_overdue":
		return installmentjob.MarkOverdue(r.logger, r.installmentlifecycleuc), nil

	case "payment_attempt_reconciliation":
		return paymentattemptjob.Reconciliation(r.paymentattemptreconciliationuc), nil

	case "merchant_terminal_issuance_scrub":
		return merchantterminaljob.IssuanceDeliveryCleanup(r.logger, r.merchantterminaluc), nil

	case "repayment_reminder_dispatch":
		if r.repaymentreminderuc == nil {
			return nil, fmt.Errorf("%s: repayment reminder use case is not configured", op)
		}

		return repaymentreminderjob.Dispatch(r.logger, r.repaymentreminderuc), nil

	default:
		return nil, fmt.Errorf("%s: unknown task handler: %q", op, handler)
	}
}
