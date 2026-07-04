package merchantrs

import "github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"

// SaleSchema is a schema-only merchant-sale list item.
type SaleSchema saleResponse

// SalesListSuccessSchema is a schema-only success envelope for listing merchant sales.
//
// Data is the sale array returned by SerKeyset or SerPage. Meta.pagination
// carries keyset or page pagination state when present.
type SalesListSuccessSchema struct {
	Data []SaleSchema        `json:"data"`
	Meta serdser.SuccessMeta `json:"meta"`
}
