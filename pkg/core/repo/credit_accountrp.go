package repo

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// CreditAccountQueryer defines read-only credit account operations.
type CreditAccountQueryer interface {
	GetByID(ctx context.Context, id model.CreditAccountID) (model.CreditAccount, error)
	GetByCustomerIDAndCurrency(
		ctx context.Context,
		customerID model.CustomerID,
		currency model.Currency,
	) (model.CreditAccount, error)
	GetByLastLimitDecisionID(
		ctx context.Context,
		decisionID model.DecisionID,
	) (model.CreditAccount, error)
	CountLimitSetTransactions(
		ctx context.Context,
		customerID model.CustomerID,
		currency model.Currency,
	) (int64, error)
}

// CreditAccountConnQueryer exposes credit account operations on a connection.
type CreditAccountConnQueryer interface {
	CreditAccountQueryer
}

// CreditAccountTxQueryer exposes transaction-scoped credit account operations.
type CreditAccountTxQueryer interface {
	CreditAccountQueryer

	LockByID(ctx context.Context, id model.CreditAccountID) (model.CreditAccount, error)
	LockByCustomerIDAndCurrency(
		ctx context.Context,
		customerID model.CustomerID,
		currency model.Currency,
	) (model.CreditAccount, error)

	// CreateWithLimit creates the single current-state account row for a customer/currency.
	CreateWithLimit(
		ctx context.Context,
		customerID model.CustomerID,
		currency model.Currency,
		limitAmountMinor uint64,
		decisionID model.DecisionID,
		effectiveFrom time.Time,
		expiresAt *time.Time,
		createdAt time.Time,
		updatedAt time.Time,
	) (model.CreditAccount, error)

	// SetLimit replaces the current governed limit state on an existing account.
	SetLimit(
		ctx context.Context,
		id model.CreditAccountID,
		limitAmountMinor uint64,
		decisionID model.DecisionID,
		effectiveFrom time.Time,
		expiresAt *time.Time,
		updatedAt time.Time,
	) (model.CreditAccount, error)

	// ExpireDueByCustomerIDAndCurrency lazily transitions a due active account to expired.
	ExpireDueByCustomerIDAndCurrency(
		ctx context.Context,
		customerID model.CustomerID,
		currency model.Currency,
		now time.Time,
	) error

	// AddExposure increases committed outstanding BNPL principal.
	AddExposure(
		ctx context.Context,
		id model.CreditAccountID,
		amountMinor uint64,
		updatedAt time.Time,
	) (model.CreditAccount, error)

	// ReduceExposureIfSufficient decreases exposure only when enough exposure exists.
	ReduceExposureIfSufficient(
		ctx context.Context,
		id model.CreditAccountID,
		amountMinor uint64,
		updatedAt time.Time,
	) (model.CreditAccount, error)

	// AppendTransaction inserts one immutable credit journal entry. The database
	// rejects duplicate business facts using (credit_account_id, type, reference_type, reference_id).
	AppendTransaction(
		ctx context.Context,
		creditAccountID model.CreditAccountID,
		type_ model.CreditTransactionType,
		amountMinor uint64,
		currency model.Currency,
		reason model.CreditTransactionReason,
		referenceType model.CreditTransactionReferenceType,
		referenceID uuid.UUID,
		occurredAt time.Time,
		createdAt time.Time,
	) (model.CreditTransaction, error)
}

// CreditAccount persists current credit-account state and its immutable journal.
type CreditAccount interface {
	Conn(Conn) CreditAccountConnQueryer
	Tx(Tx) CreditAccountTxQueryer
}
