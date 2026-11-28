package model

// MerchantOutletScope is the authoritative per-request scope resolved by Merchant.
// A selected scope with no IDs grants access to no outlets.
type MerchantOutletScope struct {
	Kind      MerchantOutletScopeKind
	OutletIDs []ExternalBranchID
}

// MerchantOutletScopeKind describes whether a grant is merchant-wide or outlet-specific.
type MerchantOutletScopeKind string

const (
	AllMerchantOutlets      MerchantOutletScopeKind = "ALL_OUTLETS"
	SelectedMerchantOutlets MerchantOutletScopeKind = "SELECTED_OUTLETS"
)

// IsValid reports whether this scope can be used to authorize a query.
func (s MerchantOutletScope) IsValid() bool {
	switch s.Kind {
	case AllMerchantOutlets:
		return len(s.OutletIDs) == 0

	case SelectedMerchantOutlets:
		for _, id := range s.OutletIDs {
			if id.IsZero() {
				return false
			}
		}
		return true

	default:
		return false
	}
}

// IsAllOutlets reports whether a merchant-wide operation is authorized by scope.
func (s MerchantOutletScope) IsAllOutlets() bool {
	return s.Kind == AllMerchantOutlets && s.IsValid()
}
