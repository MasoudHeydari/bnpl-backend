package repo

import (
	"context"
	"encoding/json"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// InstallmentChargeQueryer defines read-only installment charge operations.
type InstallmentChargeQueryer interface {
	// ListByInstallmentIDs returns charges for the given installment ids.
	ListByInstallmentIDs(
		ctx context.Context,
		installmentIDs []model.InstallmentID,
	) ([]model.InstallmentCharge, error)
}

// InstallmentChargeConnQueryer exposes installment charge operations on a connection.
type InstallmentChargeConnQueryer interface {
	// InstallmentChargeQueryer provides shared installment charge query operations.
	InstallmentChargeQueryer
}

// InstallmentChargeTxQueryer exposes transaction-scoped installment charge operations.
type InstallmentChargeTxQueryer interface {
	// LockByInstallmentIDs includes charges assessed after an attempt snapshot.
	LockByInstallmentIDs(context.Context, []model.InstallmentID) ([]model.InstallmentCharge, error)
	// InstallmentChargeQueryer provides shared installment charge query operations.
	InstallmentChargeQueryer

	// Create stores an installment charge row inside the current transaction.
	Create(
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
	) (model.InstallmentCharge, error)

	// CancelOfferedByContractID cancels offered charges for offered installments on a contract.
	CancelOfferedByContractID(
		ctx context.Context,
		contractID model.ContractID,
		updatedAt time.Time,
	) (int64, error)

	// CancelDueOfferedChargesByAllocationExpiry cancels offered charges whose parent
	// offered installment belongs to an expired checkout decision allocation.
	//
	// Charges on installments already attached to a contract are excluded.
	CancelDueOfferedChargesByAllocationExpiry(
		ctx context.Context,
		now time.Time,
		updatedAt time.Time,
	) (int64, error)

	// CancelOfferedByDecisionIDs cancels offered charges for the exact
	// decision-allocation batch claimed by the caller.
	CancelOfferedByDecisionIDs(
		ctx context.Context,
		decisionIDs []model.DecisionID,
		updatedAt time.Time,
	) (int64, error)

	// CancelDueOfferedByAllocationExpiry cancels offered charges for unattached
	// offered installments whose checkout decision allocation has expired.
	CancelDueOfferedByAllocationExpiry(
		ctx context.Context,
		now time.Time,
		updatedAt time.Time,
		limit int,
	) ([]model.InstallmentCharge, error)

	// MarkOfferedChargesAppliedByInstallmentIDs marks offered charges applied for installments.
	MarkOfferedChargesAppliedByInstallmentIDs(
		ctx context.Context,
		installmentIDs []model.InstallmentID,
		updatedAt time.Time,
	) (int64, error)

	// MarkChargeIDsApplied marks specific offered charge rows applied by id.
	MarkChargeIDsApplied(
		ctx context.Context,
		chargeIDs []model.InstallmentChargeID,
		updatedAt time.Time,
	) (int64, error)

	// MarkSnapshottedChargeIDsPaid settles specific snapshotted customer debit and credit components.
	MarkSnapshottedChargeIDsPaid(
		ctx context.Context,
		chargeIDs []model.InstallmentChargeID,
		paidAt time.Time,
		updatedAt time.Time,
	) (int64, error)

	// CountUnpaidCustomerDebitChargesByContractID counts unpaid customer debit charges on contract installments.
	CountUnpaidCustomerDebitChargesByContractID(
		ctx context.Context,
		contractID model.ContractID,
	) (int64, error)

	// LockByIDs returns and locks installment charge rows by id.
	LockByIDs(ctx context.Context, ids []model.InstallmentChargeID) ([]model.InstallmentCharge, error)
}

// InstallmentCharge persists installment charge rows.
type InstallmentCharge interface {
	// Conn binds the repository to a non-transactional connection scope.
	Conn(Conn) InstallmentChargeConnQueryer

	// Tx binds the repository to a transaction scope.
	Tx(Tx) InstallmentChargeTxQueryer
}
