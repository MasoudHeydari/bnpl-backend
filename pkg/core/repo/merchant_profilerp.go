package repo

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// MerchantProfileQueryer defines read-only merchant profile operations.
type MerchantProfileQueryer interface {
	// GetByID returns the merchant profile identified by id.
	GetByID(
		ctx context.Context,
		id model.MerchantProfileID,
	) (model.MerchantProfile, error)

	// GetByMerchantID returns the BNPL profile for a merchant identity.
	GetByMerchantID(
		ctx context.Context,
		merchantID model.MerchantID,
	) (model.MerchantProfile, error)
}

// MerchantProfileConnQueryer exposes merchant profile operations on a connection.
type MerchantProfileConnQueryer interface {
	// MerchantProfileQueryer provides shared merchant profile query operations.
	MerchantProfileQueryer
}

// MerchantProfileTxQueryer exposes transaction-scoped merchant profile operations.
type MerchantProfileTxQueryer interface {
	// MerchantProfileQueryer provides shared merchant profile query operations.
	MerchantProfileQueryer

	// Create stores a new merchant profile inside the current transaction.
	Create(
		ctx context.Context,
		merchantID model.MerchantID,
		displayName string,
		legalName string,
		logoURL string,
		supportPhone string,
		supportEmail string,
		status model.MerchantStatus,
		fundingModels []model.FundingModel,
		settlementModels []model.SettlementModel,
		currencies []model.Currency,
		riskTier model.RiskTier,
		maxAmountMinor uint64,
		maxAmountCurrency model.Currency,
		maxTenureMonths uint,
		requiresBranchMode bool,
		jurisdiction model.Jurisdiction,
		createdAt time.Time,
		updatedAt time.Time,
	) (model.MerchantProfile, error)

	// LockByID returns the merchant profile and locks it for state changes.
	LockByID(
		ctx context.Context,
		id model.MerchantProfileID,
	) (model.MerchantProfile, error)

	// LockByMerchantID returns the merchant profile for merchantID and locks it
	// for state changes.
	LockByMerchantID(
		ctx context.Context,
		merchantID model.MerchantID,
	) (model.MerchantProfile, error)

	// SetPaymentScope assigns the server-owned Financial Core payment scope for
	// a merchant. Callers must be trusted BNPL administration/provisioning —
	// never Partner HTTP handlers or merchant-controlled input. Both IDs must
	// be non-zero. Clearing scope is intentionally unsupported here so Partner
	// checkout cannot lose a previously provisioned scope through a half-update.
	SetPaymentScope(
		ctx context.Context,
		merchantID model.MerchantID,
		paymentTenantID model.TenantID,
		paymentContextID model.ContextID,
		updatedAt time.Time,
	) (model.MerchantProfile, error)
}

// MerchantProfile persists BNPL-specific merchant profile records.
type MerchantProfile interface {
	// Conn binds the repository to a non-transactional connection scope.
	Conn(Conn) MerchantProfileConnQueryer

	// Tx binds the repository to a transaction scope.
	Tx(Tx) MerchantProfileTxQueryer
}
