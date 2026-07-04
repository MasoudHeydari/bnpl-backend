package installmentchargerp

import (
	"context"
	"encoding/json"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

var _ repo.InstallmentCharge = (*Repo)(nil)

// Repo is the PostgreSQL implementation of the installment charge repository.
type Repo struct{}

// New returns a stateless PostgreSQL installment charge repository.
func New() *Repo {
	return &Repo{}
}

// Conn binds the repository to a non-transactional connection scope.
func (r *Repo) Conn(c repo.Conn) repo.InstallmentChargeConnQueryer {
	return connQueryer{Conn: postgres.MustConn(c)}
}

// Tx binds the repository to a transaction scope.
func (r *Repo) Tx(tx repo.Tx) repo.InstallmentChargeTxQueryer {
	return txQueryer{Tx: postgres.MustTx(tx)}
}

type connQueryer struct {
	*postgres.Conn
}

var _ repo.InstallmentChargeConnQueryer = (*connQueryer)(nil)

func (q connQueryer) ListByInstallmentIDs(
	ctx context.Context,
	installmentIDs []model.InstallmentID,
) ([]model.InstallmentCharge, error) {
	return ListByInstallmentIDs(ctx, q.Conn, installmentIDs)
}

type txQueryer struct {
	*postgres.Tx
}

var _ repo.InstallmentChargeTxQueryer = (*txQueryer)(nil)

func (q txQueryer) ListByInstallmentIDs(
	ctx context.Context,
	installmentIDs []model.InstallmentID,
) ([]model.InstallmentCharge, error) {
	return ListByInstallmentIDs(ctx, q.Tx, installmentIDs)
}

func (q txQueryer) Create(
	ctx context.Context,
	installmentID model.InstallmentID,
	chargeType model.InstallmentChargeType,
	chargeStatus model.InstallmentChargeStatus,
	amountMinor uint64,
	currency model.Currency,
	payerType model.InstallmentChargePayerType,
	recipientType model.InstallmentChargeRecipientType,
	policyID string,
	policyVersion string,
	assessedAt time.Time,
	dueAt *time.Time,
	paidAt *time.Time,
	metadata json.RawMessage,
	createdAt time.Time,
	updatedAt time.Time,
) (model.InstallmentCharge, error) {
	return Create(
		ctx,
		q.Tx,
		installmentID,
		chargeType,
		chargeStatus,
		amountMinor,
		currency,
		payerType,
		recipientType,
		policyID,
		policyVersion,
		assessedAt,
		dueAt,
		paidAt,
		metadata,
		createdAt,
		updatedAt,
	)
}

func (q txQueryer) CancelDueOfferedChargesByAllocationExpiry(
	ctx context.Context,
	now time.Time,
	updatedAt time.Time,
) (int64, error) {
	return CancelDueOfferedChargesByAllocationExpiry(ctx, q.Tx, now, updatedAt)
}

func (q txQueryer) CancelOfferedByDecisionIDs(
	ctx context.Context,
	decisionIDs []model.DecisionID,
	updatedAt time.Time,
) (int64, error) {
	return CancelOfferedByDecisionIDs(ctx, q.Tx, decisionIDs, updatedAt)
}

func (q txQueryer) CancelDueOfferedByAllocationExpiry(
	ctx context.Context,
	now time.Time,
	updatedAt time.Time,
	limit int,
) ([]model.InstallmentCharge, error) {
	return CancelDueOfferedByAllocationExpiry(
		ctx,
		q.Tx,
		now,
		updatedAt,
		limit,
	)
}

func (q txQueryer) CancelOfferedByContractID(
	ctx context.Context,
	contractID model.ContractID,
	updatedAt time.Time,
) (int64, error) {
	return CancelOfferedByContractID(ctx, q.Tx, contractID, updatedAt)
}

func (q txQueryer) MarkOfferedChargesAppliedByInstallmentIDs(
	ctx context.Context,
	installmentIDs []model.InstallmentID,
	updatedAt time.Time,
) (int64, error) {
	return MarkOfferedChargesAppliedByInstallmentIDs(ctx, q.Tx, installmentIDs, updatedAt)
}

func (q txQueryer) MarkChargeIDsApplied(
	ctx context.Context,
	chargeIDs []model.InstallmentChargeID,
	updatedAt time.Time,
) (int64, error) {
	return MarkChargeIDsApplied(ctx, q.Tx, chargeIDs, updatedAt)
}

func (q txQueryer) MarkSnapshottedChargeIDsPaid(
	ctx context.Context,
	chargeIDs []model.InstallmentChargeID,
	paidAt time.Time,
	updatedAt time.Time,
) (int64, error) {
	return MarkSnapshottedChargeIDsPaid(
		ctx,
		q.Tx,
		chargeIDs,
		paidAt,
		updatedAt,
	)
}

func (q txQueryer) CountUnpaidCustomerDebitChargesByContractID(
	ctx context.Context,
	contractID model.ContractID,
) (int64, error) {
	return CountUnpaidCustomerDebitChargesByContractID(ctx, q.Tx, contractID)
}

func (q txQueryer) LockByIDs(
	ctx context.Context,
	ids []model.InstallmentChargeID,
) ([]model.InstallmentCharge, error) {
	return LockByIDs(ctx, q.Tx, ids)
}

func (q txQueryer) LockByInstallmentIDs(ctx context.Context, ids []model.InstallmentID) ([]model.InstallmentCharge, error) {
	return LockByInstallmentIDs(ctx, q.Tx, ids)
}
