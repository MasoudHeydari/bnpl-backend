package creditaccountrp

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

var _ repo.CreditAccount = (*Repo)(nil)

type Repo struct{}

func New() *Repo { return &Repo{} }

func (r *Repo) Conn(c repo.Conn) repo.CreditAccountConnQueryer {
	return connQueryer{Conn: postgres.MustConn(c)}
}
func (r *Repo) Tx(tx repo.Tx) repo.CreditAccountTxQueryer { return txQueryer{Tx: postgres.MustTx(tx)} }

type connQueryer struct{ *postgres.Conn }

var _ repo.CreditAccountConnQueryer = (*connQueryer)(nil)

func (q connQueryer) GetByID(ctx context.Context, id model.CreditAccountID) (model.CreditAccount, error) {
	return GetByID(ctx, q.Conn, id)
}
func (q connQueryer) GetByCustomerIDAndCurrency(ctx context.Context, customerID model.CustomerID, currency model.Currency) (model.CreditAccount, error) {
	return GetByCustomerIDAndCurrency(ctx, q.Conn, customerID, currency)
}
func (q connQueryer) GetByLastLimitDecisionID(ctx context.Context, decisionID model.DecisionID) (model.CreditAccount, error) {
	return GetByLastLimitDecisionID(ctx, q.Conn, decisionID)
}
func (q connQueryer) CountLimitSetTransactions(ctx context.Context, customerID model.CustomerID, currency model.Currency) (int64, error) {
	return CountLimitSetTransactions(ctx, q.Conn, customerID, currency)
}

type txQueryer struct{ *postgres.Tx }

var _ repo.CreditAccountTxQueryer = (*txQueryer)(nil)

func (q txQueryer) GetByID(ctx context.Context, id model.CreditAccountID) (model.CreditAccount, error) {
	return GetByID(ctx, q.Tx, id)
}
func (q txQueryer) GetByCustomerIDAndCurrency(ctx context.Context, customerID model.CustomerID, currency model.Currency) (model.CreditAccount, error) {
	return GetByCustomerIDAndCurrency(ctx, q.Tx, customerID, currency)
}
func (q txQueryer) GetByLastLimitDecisionID(ctx context.Context, decisionID model.DecisionID) (model.CreditAccount, error) {
	return GetByLastLimitDecisionID(ctx, q.Tx, decisionID)
}
func (q txQueryer) CountLimitSetTransactions(ctx context.Context, customerID model.CustomerID, currency model.Currency) (int64, error) {
	return CountLimitSetTransactions(ctx, q.Tx, customerID, currency)
}
func (q txQueryer) LockByID(ctx context.Context, id model.CreditAccountID) (model.CreditAccount, error) {
	return LockByID(ctx, q.Tx, id)
}
func (q txQueryer) LockByCustomerIDAndCurrency(ctx context.Context, customerID model.CustomerID, currency model.Currency) (model.CreditAccount, error) {
	return LockByCustomerIDAndCurrency(ctx, q.Tx, customerID, currency)
}
func (q txQueryer) CreateWithLimit(
	ctx context.Context,
	customerID model.CustomerID,
	currency model.Currency,
	limitAmountMinor uint64,
	decisionID model.DecisionID,
	effectiveFrom time.Time,
	expiresAt *time.Time,
	createdAt time.Time,
	updatedAt time.Time,
) (model.CreditAccount, error) {
	return CreateWithLimit(
		ctx,
		q.Tx,
		customerID,
		currency,
		limitAmountMinor,
		decisionID,
		effectiveFrom,
		expiresAt,
		createdAt,
		updatedAt,
	)
}
func (q txQueryer) SetLimit(
	ctx context.Context,
	id model.CreditAccountID,
	limitAmountMinor uint64,
	decisionID model.DecisionID,
	effectiveFrom time.Time,
	expiresAt *time.Time,
	updatedAt time.Time,
) (model.CreditAccount, error) {
	return SetLimit(
		ctx,
		q.Tx,
		id,
		limitAmountMinor,
		decisionID,
		effectiveFrom,
		expiresAt,
		updatedAt,
	)
}
func (q txQueryer) ExpireDueByCustomerIDAndCurrency(ctx context.Context, customerID model.CustomerID, currency model.Currency, now time.Time) error {
	return ExpireDueByCustomerIDAndCurrency(
		ctx,
		q.Tx,
		customerID,
		currency,
		now,
	)
}
func (q txQueryer) AddExposure(ctx context.Context, id model.CreditAccountID, amountMinor uint64, updatedAt time.Time) (model.CreditAccount, error) {
	return AddExposure(
		ctx,
		q.Tx,
		id,
		amountMinor,
		updatedAt,
	)
}
func (q txQueryer) ReduceExposureIfSufficient(ctx context.Context, id model.CreditAccountID, amountMinor uint64, updatedAt time.Time) (model.CreditAccount, error) {
	return ReduceExposureIfSufficient(
		ctx,
		q.Tx,
		id,
		amountMinor,
		updatedAt,
	)
}
func (q txQueryer) AppendTransaction(
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
) (model.CreditTransaction, error) {
	return AppendTransaction(
		ctx,
		q.Tx,
		creditAccountID,
		type_,
		amountMinor,
		currency,
		reason,
		referenceType,
		referenceID,
		occurredAt,
		createdAt,
	)
}

// Product-aware methods are additive: legacy account operations continue to
// address only pay-in-four, while callers can explicitly request monthly credit.
var _ repo.CreditAccountProductQueryer = (*connQueryer)(nil)
var _ repo.CreditAccountProductTxQueryer = (*txQueryer)(nil)

func (q connQueryer) GetByCustomerIDCurrencyAndKind(ctx context.Context, id model.CustomerID, currency model.Currency, kind model.CreditKind) (model.CreditAccount, error) {
	return GetByCustomerIDCurrencyAndKind(ctx, q.Conn, id, currency, kind)
}
func (q connQueryer) ListByCustomerID(ctx context.Context, id model.CustomerID) ([]model.CreditAccount, error) {
	return ListByCustomerID(ctx, q.Conn, id)
}
func (q txQueryer) GetByCustomerIDCurrencyAndKind(ctx context.Context, id model.CustomerID, currency model.Currency, kind model.CreditKind) (model.CreditAccount, error) {
	return GetByCustomerIDCurrencyAndKind(ctx, q.Tx, id, currency, kind)
}
func (q txQueryer) ListByCustomerID(ctx context.Context, id model.CustomerID) ([]model.CreditAccount, error) {
	return ListByCustomerID(ctx, q.Tx, id)
}
func (q txQueryer) LockByCustomerIDCurrencyAndKind(ctx context.Context, id model.CustomerID, currency model.Currency, kind model.CreditKind) (model.CreditAccount, error) {
	return LockByCustomerIDCurrencyAndKind(ctx, q.Tx, id, currency, kind)
}
func (q txQueryer) CreateWithLimitForKind(ctx context.Context, id model.CustomerID, currency model.Currency, kind model.CreditKind,
	limit uint64, decision model.DecisionID, from time.Time, expires *time.Time, created time.Time, updated time.Time) (model.CreditAccount, error) {
	return CreateWithLimitForKind(ctx, q.Tx, id, currency, kind, limit, decision, from, expires, created, updated)
}
func (q txQueryer) ExpireDueByCustomerIDCurrencyAndKind(ctx context.Context, id model.CustomerID, currency model.Currency, kind model.CreditKind, now time.Time) error {
	return ExpireDueByCustomerIDCurrencyAndKind(ctx, q.Tx, id, currency, kind, now)
}

func (q connQueryer) HasUnpaidMonthlyCycleBefore(
	ctx context.Context, id model.CustomerID, currency model.Currency, cutoff time.Time,
) (bool, error) {
	return HasUnpaidMonthlyCycleBefore(ctx, q.Conn, id, currency, cutoff)
}
func (q txQueryer) HasUnpaidMonthlyCycleBefore(
	ctx context.Context, id model.CustomerID, currency model.Currency, cutoff time.Time,
) (bool, error) {
	return HasUnpaidMonthlyCycleBefore(ctx, q.Tx, id, currency, cutoff)
}
