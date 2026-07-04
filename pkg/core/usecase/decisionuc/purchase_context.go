package decisionuc

import (
	"context"

	"github.com/google/uuid"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

const maxMerchantOrderIDLength = 256

// MaxMerchantOrderIDLength is the maximum accepted length for an optional
// merchant-owned online order identifier.
const MaxMerchantOrderIDLength = maxMerchantOrderIDLength

// PurchaseContext identifies the merchant sales surface used for checkout.
// Exactly one source identifier is populated according to Channel.
type PurchaseContext struct {
	// Channel selects physical POS or online-store resolution.
	Channel model.PurchaseChannel
	// PosGpsID addresses a physical POS terminal when Channel is pos.
	PosGpsID model.PosGpsID
	// OnlineStoreID addresses an online storefront when Channel is online.
	OnlineStoreID model.OnlineStoreID
	// MerchantOrderID optionally correlates an online BNPL checkout to the merchant's own order.
	MerchantOrderID string
}

// Validate reports whether the purchase context shape is client-safe.
//
// The HTTP adapter calls Validate for fail-fast rejection. Create repeats the
// same invariants because callers are not limited to the HTTP adapter.
func (p PurchaseContext) Validate() error {
	return p.validate()
}

// sourceID returns the single external source UUID selected by Channel.
func (p PurchaseContext) sourceID() uuid.UUID {
	switch p.Channel {
	case model.POSPurchaseChannel:
		return p.PosGpsID.UUID()

	case model.OnlinePurchaseChannel:
		return p.OnlineStoreID.UUID()

	default:
		return uuid.Nil
	}
}

// merchantOrderIDPointer returns nil when no merchant-owned online order was supplied.
func (p PurchaseContext) merchantOrderIDPointer() *string {
	if p.MerchantOrderID == "" {
		return nil
	}

	value := p.MerchantOrderID

	return &value
}

// validate enforces an explicit, non-ambiguous purchase source. Checkout must
// never infer the channel from the presence or absence of optional fields.
func (p PurchaseContext) validate() error {
	if !p.Channel.IsValid() {
		return purchaseChannelInvalidError()
	}

	switch p.Channel {
	case model.POSPurchaseChannel:
		if p.PosGpsID.IsZero() {
			return posGpsIDRequiredError()
		}

		if !p.OnlineStoreID.IsZero() {
			return onlineStoreIDNotAllowedError()
		}

		if p.MerchantOrderID != "" {
			return merchantOrderIDNotAllowedError()
		}

	case model.OnlinePurchaseChannel:
		if p.OnlineStoreID.IsZero() {
			return onlineStoreIDRequiredError()
		}

		if !p.PosGpsID.IsZero() {
			return posGpsIDNotAllowedError()
		}

		if len(p.MerchantOrderID) > maxMerchantOrderIDLength {
			return merchantOrderIDTooLongError()
		}
	}

	return nil
}

// resolveMerchantBranch locks and returns the selling point mapped to this
// purchase source. POS mappings can only resolve physical selling points and
// online-store mappings can only resolve online selling points.
func (p PurchaseContext) resolveMerchantBranch(
	ctx context.Context,
	branchQ repo.MerchantBranchProfileTxQueryer,
) (model.MerchantBranchProfile, error) {
	switch p.Channel {
	case model.POSPurchaseChannel:
		return branchQ.LockByPosGpsID(ctx, p.PosGpsID)

	case model.OnlinePurchaseChannel:
		return branchQ.LockByOnlineStoreID(ctx, p.OnlineStoreID)

	default:
		return model.MerchantBranchProfile{}, purchaseChannelInvalidError()
	}
}

func purchaseContextFromAllocation(allocation model.CheckoutDecisionAllocation) PurchaseContext {
	context := PurchaseContext{
		Channel: allocation.PurchaseChannel,
	}
	if allocation.MerchantOrderID != nil {
		context.MerchantOrderID = *allocation.MerchantOrderID
	}

	switch allocation.PurchaseChannel {
	case model.POSPurchaseChannel:
		context.PosGpsID = model.PosGpsID(allocation.PurchaseSourceID)

	case model.OnlinePurchaseChannel:
		context.OnlineStoreID = model.OnlineStoreID(allocation.PurchaseSourceID)
	}

	return context
}
