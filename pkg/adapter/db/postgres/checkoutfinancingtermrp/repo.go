package checkoutfinancingtermrp

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// Verify Repo implements the core checkout financing terms repository contract.
var _ repo.CheckoutFinancingTerm = (*Repo)(nil)

// Repo is the PostgreSQL implementation of the checkout financing terms repository.
type Repo struct{}

// New returns a stateless PostgreSQL checkout financing terms repository.
func New() *Repo {
	return &Repo{}
}

// Conn binds the repository to a non-transactional connection scope.
func (r *Repo) Conn(c repo.Conn) repo.CheckoutFinancingTermConnQueryer {
	return connQueryer{Conn: c.(*postgres.Conn)}
}

// Tx binds the repository to a transaction scope.
func (r *Repo) Tx(tx repo.Tx) repo.CheckoutFinancingTermTxQueryer {
	return txQueryer{Tx: tx.(*postgres.Tx)}
}

type connQueryer struct {
	*postgres.Conn
}

var _ repo.CheckoutFinancingTermConnQueryer = (*connQueryer)(nil)

func (q connQueryer) GetByID(
	ctx context.Context,
	id uuid.UUID,
) (model.CheckoutFinancingTerm, error) {
	return GetByID(ctx, q.Conn, id)
}

func (q connQueryer) GetByDecisionID(
	ctx context.Context,
	decisionID uuid.UUID,
) (model.CheckoutFinancingTerm, error) {
	return GetByDecisionID(ctx, q.Conn, decisionID)
}

func (q connQueryer) GetByCheckoutIntentID(
	ctx context.Context,
	checkoutIntentID uuid.UUID,
) (model.CheckoutFinancingTerm, error) {
	return GetByCheckoutIntentID(ctx, q.Conn, checkoutIntentID)
}

func (q connQueryer) GetOfferedByCheckoutIntentID(
	ctx context.Context,
	checkoutIntentID uuid.UUID,
) (model.CheckoutFinancingTerm, error) {
	return GetOfferedByCheckoutIntentID(ctx, q.Conn, checkoutIntentID)
}

type txQueryer struct {
	*postgres.Tx
}

var _ repo.CheckoutFinancingTermTxQueryer = (*txQueryer)(nil)

func (q txQueryer) GetByID(
	ctx context.Context,
	id uuid.UUID,
) (model.CheckoutFinancingTerm, error) {
	return GetByID(ctx, q.Tx, id)
}

func (q txQueryer) GetByDecisionID(
	ctx context.Context,
	decisionID uuid.UUID,
) (model.CheckoutFinancingTerm, error) {
	return GetByDecisionID(ctx, q.Tx, decisionID)
}

func (q txQueryer) GetByCheckoutIntentID(
	ctx context.Context,
	checkoutIntentID uuid.UUID,
) (model.CheckoutFinancingTerm, error) {
	return GetByCheckoutIntentID(ctx, q.Tx, checkoutIntentID)
}

func (q txQueryer) GetOfferedByCheckoutIntentID(
	ctx context.Context,
	checkoutIntentID uuid.UUID,
) (model.CheckoutFinancingTerm, error) {
	return GetOfferedByCheckoutIntentID(ctx, q.Tx, checkoutIntentID)
}

func (q txQueryer) Create(
	ctx context.Context,
	checkoutIntentID uuid.UUID,
	decisionID uuid.UUID,
	borrowerProfileID uuid.UUID,
	merchantProfileID uuid.UUID,
	merchantBranchProfileID *uuid.UUID,
	requestedAmountMinor uint64,
	financedAmountMinor uint64,
	upfrontAmountMinor uint64,
	regularInstallmentAmountMinor uint64,
	firstInstallmentAmountMinor uint64,
	regularInstallmentPrincipalAmountMinor uint64,
	firstInstallmentPrincipalAmountMinor uint64,
	firstInstallmentUpfrontAmountMinor uint64,
	firstInstallmentTotalAmountMinor uint64,
	totalFeeAmountMinor uint64,
	totalPayableAmountMinor uint64,
	currency model.Currency,
	installmentCount int,
	status model.CheckoutFinancingTermStatus,
	expiresAt time.Time,
	createdAt time.Time,
	updatedAt time.Time,
) (model.CheckoutFinancingTerm, error) {
	return Create(
		ctx,
		q.Tx,
		checkoutIntentID,
		decisionID,
		borrowerProfileID,
		merchantProfileID,
		merchantBranchProfileID,
		requestedAmountMinor,
		financedAmountMinor,
		upfrontAmountMinor,
		regularInstallmentAmountMinor,
		firstInstallmentAmountMinor,
		regularInstallmentPrincipalAmountMinor,
		firstInstallmentPrincipalAmountMinor,
		firstInstallmentUpfrontAmountMinor,
		firstInstallmentTotalAmountMinor,
		totalFeeAmountMinor,
		totalPayableAmountMinor,
		currency,
		installmentCount,
		status,
		expiresAt,
		createdAt,
		updatedAt,
	)
}

func (q txQueryer) ExpireDueOfferedByBorrowerProfileIDAndCurrency(
	ctx context.Context,
	borrowerProfileID uuid.UUID,
	currency model.Currency,
	now time.Time,
	updatedAt time.Time,
) error {
	return ExpireDueOfferedByBorrowerProfileIDAndCurrency(
		ctx,
		q.Tx,
		borrowerProfileID,
		currency,
		now,
		updatedAt,
	)
}
