package repo

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/pagination"
)

// InstallmentQueryer defines read-only installment operations.
type InstallmentQueryer interface {
	// GetByID returns an installment by id.
	GetByID(ctx context.Context, id model.InstallmentID) (model.Installment, error)

	// ListByContractID returns installments for a contract ordered by installment_order.
	ListByContractID(ctx context.Context, contractID model.ContractID) ([]model.Installment, error)

	// ListByDecisionID returns installments for a decision ordered by installment_order.
	ListByDecisionID(ctx context.Context, decisionID model.DecisionID) ([]model.Installment, error)

	// ListUnpaidByCustomerID returns unpaid contract installments for a
	// customer profile using keyset pagination ordered by due_at ASC, id ASC.
	//
	// Unpaid means status in scheduled, due, or overdue, with contract_id set.
	ListUnpaidByCustomerID(
		ctx context.Context,
		customerID model.CustomerID,
		page pagination.KeysetRequest,
	) ([]model.Installment, error)

	// ListReminderCandidatePage returns all eligible reminder installments for
	// the next customerLimit customers after afterCustomerID (exclusive keyset;
	// a zero afterCustomerID starts from the beginning).
	//
	// Eligibility: unpaid statuses on active contracts with due_at in
	// [dueAtFrom, dueAtTo). Customers who already have a repayment reminder for
	// businessDate are excluded so repeated runs advance past processed days.
	//
	// Results never truncate mid-customer: every matching installment for each
	// selected customer is returned, ordered by customer_id, due_at, id.
	// Results are enriched with ExternalCustomerID and whether a non-terminal
	// installment repayment / checkout-first payment attempt currently exists.
	ListReminderCandidatePage(
		ctx context.Context,
		dueAtFrom time.Time,
		dueAtTo time.Time,
		businessDate time.Time,
		afterCustomerID model.CustomerID,
		customerLimit int,
	) ([]ReminderCandidate, error)
}

// ReminderCandidate is an unpaid installment considered for repayment reminder
// evaluation, enriched with recipient and payment-in-flight facts.
type ReminderCandidate struct {
	// Installment is the unpaid schedule installment.
	Installment model.Installment

	// ExternalCustomerID is the Identity / Customer Reference identifier for SMS.
	ExternalCustomerID model.ExternalCustomerID

	// HasUnresolvedPayment is true when a non-terminal payment attempt exists
	// for the installment.
	HasUnresolvedPayment bool
}

// InstallmentConnQueryer exposes installment operations on a connection.
type InstallmentConnQueryer interface {
	// InstallmentQueryer provides shared installment query operations.
	InstallmentQueryer
}

// InstallmentTxQueryer exposes transaction-scoped installment operations.
type InstallmentTxQueryer interface {
	// InstallmentQueryer provides shared installment query operations.
	InstallmentQueryer

	// LockByDecisionID returns and locks installments for a decision ordered by installment_order.
	LockByDecisionID(
		ctx context.Context,
		decisionID model.DecisionID,
	) ([]model.Installment, error)

	// LockByID returns and locks an installment row for update.
	LockByID(ctx context.Context, installmentID model.InstallmentID) (model.Installment, error)

	// LockByContractID returns and locks all installments attached to a contract.
	// Contract-scoped workflows use this after locking the Contract row to
	// serialize against installment lifecycle jobs without reversing lock order.
	LockByContractID(ctx context.Context, contractID model.ContractID) ([]model.Installment, error)

	// AttachContractToOfferedByDecisionID sets contract_id on offered installments for a decision.
	AttachContractToOfferedByDecisionID(
		ctx context.Context,
		decisionID model.DecisionID,
		contractID model.ContractID,
		updatedAt time.Time,
	) (int64, error)

	// MarkPaid marks an installment paid.
	MarkPaid(
		ctx context.Context,
		installmentID model.InstallmentID,
		contractID model.ContractID,
		paidAmountMinor uint64,
		paidAt time.Time,
		updatedAt time.Time,
	) (model.Installment, error)

	// UpdateOfferedDueAt finalizes an offered installment due date before activation.
	// It refuses to mutate installments that have already left the offered state.
	UpdateOfferedDueAt(
		ctx context.Context,
		installmentID model.InstallmentID,
		dueAt time.Time,
		updatedAt time.Time,
	) (model.Installment, error)

	// MarkScheduledOrDue transitions an offered installment to scheduled or due.
	MarkScheduledOrDue(
		ctx context.Context,
		installmentID model.InstallmentID,
		contractID model.ContractID,
		nextStatus model.InstallmentStatus,
		updatedAt time.Time,
	) (model.Installment, error)

	// Create stores an installment row inside the current transaction.
	Create(
		ctx context.Context,
		decisionID model.DecisionID,
		contractID *model.ContractID,
		customerID model.CustomerID,
		merchantProfileID model.MerchantProfileID,
		merchantBranchProfileID *model.MerchantBranchProfileID,
		installmentOrder int,
		currency model.Currency,
		principalAmountMinor uint64,
		upfrontAmountMinor uint64,
		baseFeeAmountMinor uint64,
		totalAmountMinor uint64,
		dueAt time.Time,
		gracePeriodDays uint,
		status model.InstallmentStatus,
		paidAmountMinor uint64,
		paidAt *time.Time,
		createdAt time.Time,
		updatedAt time.Time,
	) (model.Installment, error)

	// ClaimScheduledDue locks scheduled installments whose due_at has passed.
	ClaimScheduledDue(
		ctx context.Context,
		now time.Time,
		limit int,
	) ([]model.Installment, error)

	// MarkDue transitions a scheduled installment to due.
	MarkDue(
		ctx context.Context,
		installmentID model.InstallmentID,
		updatedAt time.Time,
	) (model.Installment, error)

	// ClaimDueOverdueEligible locks due installments whose grace period has elapsed.
	ClaimDueOverdueEligible(
		ctx context.Context,
		now time.Time,
		limit int,
	) ([]model.Installment, error)

	// MarkOverdue transitions a due installment to overdue.
	MarkOverdue(
		ctx context.Context,
		installmentID model.InstallmentID,
		updatedAt time.Time,
	) (model.Installment, error)

	// ExpireOfferedByContractID marks offered installments expired for a contract.
	ExpireOfferedByContractID(
		ctx context.Context,
		contractID model.ContractID,
		updatedAt time.Time,
	) (int64, error)

	// ExpireDueOfferedByAllocationExpiry marks offered installments expired when
	// their parent checkout decision allocation has expired.
	//
	// Installments already attached to a contract are excluded.
	ExpireDueOfferedByAllocationExpiry(
		ctx context.Context,
		now time.Time,
		updatedAt time.Time,
		limit int,
	) ([]model.Installment, error)

	// ExpireOfferedByDecisionIDs expires unattached offered installments for an
	// exact decision-allocation batch claimed by the caller.
	ExpireOfferedByDecisionIDs(
		ctx context.Context,
		decisionIDs []model.DecisionID,
		updatedAt time.Time,
	) (int64, error)
}

// Installment persists checkout installment offer rows.
type Installment interface {
	// Conn binds the repository to a non-transactional connection scope.
	Conn(Conn) InstallmentConnQueryer

	// Tx binds the repository to a transaction scope.
	Tx(Tx) InstallmentTxQueryer
}
