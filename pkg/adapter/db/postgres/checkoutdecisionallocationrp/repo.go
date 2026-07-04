package checkoutdecisionallocationrp

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

var _ repo.CheckoutDecisionAllocation = (*Repo)(nil)

// Repo is the PostgreSQL implementation of the checkout decision allocation repository.
type Repo struct{}

// New returns a stateless PostgreSQL checkout decision allocation repository.
func New() *Repo {
	return &Repo{}
}

// Conn binds the repository to a non-transactional connection scope.
func (r *Repo) Conn(c repo.Conn) repo.CheckoutDecisionAllocationConnQueryer {
	return connQueryer{Conn: postgres.MustConn(c)}
}

// Tx binds the repository to a transaction scope.
func (r *Repo) Tx(tx repo.Tx) repo.CheckoutDecisionAllocationTxQueryer {
	return txQueryer{Tx: postgres.MustTx(tx)}
}

type connQueryer struct {
	*postgres.Conn
}

var _ repo.CheckoutDecisionAllocationConnQueryer = (*connQueryer)(nil)

func (q connQueryer) GetByDecisionID(
	ctx context.Context,
	decisionID model.DecisionID,
) (model.CheckoutDecisionAllocation, error) {
	return GetByDecisionID(ctx, q.Conn, decisionID)
}

type txQueryer struct {
	*postgres.Tx
}

var _ repo.CheckoutDecisionAllocationTxQueryer = (*txQueryer)(nil)

func (q txQueryer) GetByDecisionID(
	ctx context.Context,
	decisionID model.DecisionID,
) (model.CheckoutDecisionAllocation, error) {
	return GetByDecisionID(ctx, q.Tx, decisionID)
}

func (q txQueryer) LockByDecisionID(
	ctx context.Context,
	decisionID model.DecisionID,
) (model.CheckoutDecisionAllocation, error) {
	return LockByDecisionID(ctx, q.Tx, decisionID)
}

func (q txQueryer) Create(
	ctx context.Context,
	decisionID model.DecisionID,
	checkoutKey string,
	customerID model.CustomerID,
	merchantProfileID model.MerchantProfileID,
	merchantBranchProfileID *model.MerchantBranchProfileID,
	purchaseChannel model.PurchaseChannel,
	purchaseSourceID uuid.UUID,
	merchantOrderID *string,
	requestedAmountMinor uint64,
	approvedFinancedAmountMinor uint64,
	upfrontAmountMinor uint64,
	availableCreditMinor uint64,
	currency model.Currency,
	installmentCount int,
	expiresAt time.Time,
	createdAt time.Time,
) (model.CheckoutDecisionAllocation, error) {
	return Create(
		ctx,
		q.Tx,
		decisionID,
		checkoutKey,
		customerID,
		merchantProfileID,
		merchantBranchProfileID,
		purchaseChannel,
		purchaseSourceID,
		merchantOrderID,
		requestedAmountMinor,
		approvedFinancedAmountMinor,
		upfrontAmountMinor,
		availableCreditMinor,
		currency,
		installmentCount,
		expiresAt,
		createdAt,
	)
}
