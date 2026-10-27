package installmentrp

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/pagination"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

var _ repo.Installment = (*Repo)(nil)

// Repo is the PostgreSQL implementation of the installment repository.
type Repo struct{}

// New returns a stateless PostgreSQL installment repository.
func New() *Repo {
	return &Repo{}
}

// Conn binds the repository to a non-transactional connection scope.
func (r *Repo) Conn(c repo.Conn) repo.InstallmentConnQueryer {
	return connQueryer{Conn: postgres.MustConn(c)}
}

// Tx binds the repository to a transaction scope.
func (r *Repo) Tx(tx repo.Tx) repo.InstallmentTxQueryer {
	return txQueryer{Tx: postgres.MustTx(tx)}
}

type connQueryer struct {
	*postgres.Conn
}

var _ repo.InstallmentConnQueryer = (*connQueryer)(nil)

func (q connQueryer) GetByID(ctx context.Context, id model.InstallmentID) (model.Installment, error) {
	return GetByID(ctx, q.Conn, id)
}

func (q connQueryer) ListByContractID(
	ctx context.Context,
	contractID model.ContractID,
) ([]model.Installment, error) {
	return ListByContractID(ctx, q.Conn, contractID)
}

func (q connQueryer) ListByDecisionID(
	ctx context.Context,
	decisionID model.DecisionID,
) ([]model.Installment, error) {
	return ListByDecisionID(ctx, q.Conn, decisionID)
}

func (q connQueryer) ListUnpaidByCustomerID(
	ctx context.Context,
	customerID model.CustomerID,
	page pagination.KeysetRequest,
) ([]model.Installment, error) {
	return ListUnpaidByCustomerID(ctx, q.Conn, customerID, page)
}

func (q connQueryer) ListReminderCandidatePage(
	ctx context.Context,
	dueAtFrom time.Time,
	dueAtTo time.Time,
	businessDate time.Time,
	afterCustomerID model.CustomerID,
	customerLimit int,
) ([]repo.ReminderCandidate, error) {
	return ListReminderCandidatePage(
		ctx,
		q.Conn,
		dueAtFrom,
		dueAtTo,
		businessDate,
		afterCustomerID,
		customerLimit,
	)
}

type txQueryer struct {
	*postgres.Tx
}

var _ repo.InstallmentTxQueryer = (*txQueryer)(nil)

func (q txQueryer) GetByID(ctx context.Context, id model.InstallmentID) (model.Installment, error) {
	return GetByID(ctx, q.Tx, id)
}

func (q txQueryer) ListByContractID(
	ctx context.Context,
	contractID model.ContractID,
) ([]model.Installment, error) {
	return ListByContractID(ctx, q.Tx, contractID)
}

func (q txQueryer) ListByDecisionID(
	ctx context.Context,
	decisionID model.DecisionID,
) ([]model.Installment, error) {
	return ListByDecisionID(ctx, q.Tx, decisionID)
}

func (q txQueryer) ListUnpaidByCustomerID(
	ctx context.Context,
	customerID model.CustomerID,
	page pagination.KeysetRequest,
) ([]model.Installment, error) {
	return ListUnpaidByCustomerID(ctx, q.Tx, customerID, page)
}

func (q txQueryer) ListReminderCandidatePage(
	ctx context.Context,
	dueAtFrom time.Time,
	dueAtTo time.Time,
	businessDate time.Time,
	afterCustomerID model.CustomerID,
	customerLimit int,
) ([]repo.ReminderCandidate, error) {
	return ListReminderCandidatePage(
		ctx,
		q.Tx,
		dueAtFrom,
		dueAtTo,
		businessDate,
		afterCustomerID,
		customerLimit,
	)
}

func (q txQueryer) LockByID(
	ctx context.Context,
	installmentID model.InstallmentID,
) (model.Installment, error) {
	return LockByID(ctx, q.Tx, installmentID)
}

func (q txQueryer) LockByContractID(
	ctx context.Context,
	contractID model.ContractID,
) ([]model.Installment, error) {
	return LockByContractID(ctx, q.Tx, contractID)
}

func (q txQueryer) LockByDecisionID(
	ctx context.Context,
	decisionID model.DecisionID,
) ([]model.Installment, error) {
	return LockByDecisionID(ctx, q.Tx, decisionID)
}

func (q txQueryer) AttachContractToOfferedByDecisionID(
	ctx context.Context,
	decisionID model.DecisionID,
	contractID model.ContractID,
	updatedAt time.Time,
) (int64, error) {
	return AttachContractToOfferedByDecisionID(
		ctx,
		q.Tx,
		decisionID,
		contractID,
		updatedAt,
	)
}

func (q txQueryer) MarkPaid(
	ctx context.Context,
	installmentID model.InstallmentID,
	contractID model.ContractID,
	paidAmountMinor uint64,
	paidAt time.Time,
	updatedAt time.Time,
) (model.Installment, error) {
	return MarkPaid(
		ctx,
		q.Tx,
		installmentID,
		contractID,
		paidAmountMinor,
		paidAt,
		updatedAt,
	)
}

func (q txQueryer) UpdateOfferedDueAt(
	ctx context.Context,
	installmentID model.InstallmentID,
	dueAt time.Time,
	updatedAt time.Time,
) (model.Installment, error) {
	return UpdateOfferedDueAt(
		ctx,
		q.Tx,
		installmentID,
		dueAt,
		updatedAt,
	)
}

func (q txQueryer) MarkScheduledOrDue(
	ctx context.Context,
	installmentID model.InstallmentID,
	contractID model.ContractID,
	nextStatus model.InstallmentStatus,
	updatedAt time.Time,
) (model.Installment, error) {
	return MarkScheduledOrDue(
		ctx,
		q.Tx,
		installmentID,
		contractID,
		nextStatus,
		updatedAt,
	)
}

func (q txQueryer) Create(
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
) (model.Installment, error) {
	return Create(
		ctx,
		q.Tx,
		decisionID,
		contractID,
		customerID,
		merchantProfileID,
		merchantBranchProfileID,
		installmentOrder,
		currency,
		principalAmountMinor,
		upfrontAmountMinor,
		baseFeeAmountMinor,
		totalAmountMinor,
		dueAt,
		gracePeriodDays,
		status,
		paidAmountMinor,
		paidAt,
		createdAt,
		updatedAt,
	)
}

func (q txQueryer) ExpireDueOfferedByAllocationExpiry(
	ctx context.Context,
	now time.Time,
	updatedAt time.Time,
	limit int,
) ([]model.Installment, error) {
	return ExpireDueOfferedByAllocationExpiry(
		ctx,
		q.Tx,
		now,
		updatedAt,
		limit,
	)
}

func (q txQueryer) ExpireOfferedByDecisionIDs(
	ctx context.Context,
	decisionIDs []model.DecisionID,
	updatedAt time.Time,
) (int64, error) {
	return ExpireOfferedByDecisionIDs(ctx, q.Tx, decisionIDs, updatedAt)
}

func (q txQueryer) ClaimScheduledDue(
	ctx context.Context,
	now time.Time,
	limit int,
) ([]model.Installment, error) {
	return ClaimScheduledDue(ctx, q.Tx, now, limit)
}

func (q txQueryer) MarkDue(
	ctx context.Context,
	installmentID model.InstallmentID,
	updatedAt time.Time,
) (model.Installment, error) {
	return MarkDue(ctx, q.Tx, installmentID, updatedAt)
}

func (q txQueryer) ClaimDueOverdueEligible(
	ctx context.Context,
	now time.Time,
	limit int,
) ([]model.Installment, error) {
	return ClaimDueOverdueEligible(ctx, q.Tx, now, limit)
}

func (q txQueryer) MarkOverdue(
	ctx context.Context,
	installmentID model.InstallmentID,
	updatedAt time.Time,
) (model.Installment, error) {
	return MarkOverdue(ctx, q.Tx, installmentID, updatedAt)
}

func (q txQueryer) ExpireOfferedByContractID(
	ctx context.Context,
	contractID model.ContractID,
	updatedAt time.Time,
) (int64, error) {
	return ExpireOfferedByContractID(ctx, q.Tx, contractID, updatedAt)
}
