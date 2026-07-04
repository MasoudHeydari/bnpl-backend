package model

// PurchaseChannel identifies how a merchant sale reaches BNPL checkout.
//
// Channel describes the sales surface, not the financing policy. The same
// checkout policy may therefore be used for both physical POS and online
// purchases.
type PurchaseChannel string

const (
	// POSPurchaseChannel identifies an in-person purchase addressed by a POS/GPS terminal identity.
	POSPurchaseChannel PurchaseChannel = "pos"

	// OnlinePurchaseChannel identifies an online purchase addressed by an online-store identity.
	OnlinePurchaseChannel PurchaseChannel = "online"
)

// IsValid reports whether the purchase channel is supported by checkout.
func (c PurchaseChannel) IsValid() bool {
	return c == POSPurchaseChannel || c == OnlinePurchaseChannel
}
