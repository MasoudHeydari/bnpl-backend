package decisionrs

import "github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"

// CreateRequestSchema is a schema-only representation of the create request body.
// It matches createRequest JSON exactly and contains no business logic.
//
// purchase_context is a discriminator on type:
//   - type=pos requires pos_gps_id; online_store_id and merchant_order_id are forbidden
//   - type=online requires online_store_id; pos_gps_id is forbidden; merchant_order_id is optional
//
// swag v1 does not emit OpenAPI oneOf/discriminator for this nested object cleanly,
// so the contract is documented here and on the Create endpoint descriptions.
type CreateRequestSchema createRequest

// POSPurchaseContextSchema documents the POS variant of purchase_context.
//
//	@Description	POS purchase context: type must be "pos", pos_gps_id is required, online_store_id and merchant_order_id must be omitted.
type POSPurchaseContextSchema struct {
	Type     string `json:"type" binding:"required" example:"pos" enums:"pos"`
	PosGpsID string `json:"pos_gps_id" binding:"required" example:"6ba7b811-9dad-11d1-80b4-00c04fd430c8" format:"uuid"`
}

// OnlinePurchaseContextSchema documents the online variant of purchase_context.
//
//	@Description	Online purchase context: type must be "online", online_store_id is required, pos_gps_id must be omitted, merchant_order_id is optional.
type OnlinePurchaseContextSchema struct {
	Type            string `json:"type" binding:"required" example:"online" enums:"online"`
	OnlineStoreID   string `json:"online_store_id" binding:"required" example:"8e1a2b3c-4d5e-4f6a-8b7c-9d0e1f2a3b4c" format:"uuid"`
	MerchantOrderID string `json:"merchant_order_id,omitempty" example:"ORD-2026-847291" maxLength:"256"`
}

// DecisionSchema is a schema-only representation of the decision payload.
type DecisionSchema decisionResponse

// CreateResponseSchema is a schema-only representation of the create response data.
type CreateResponseSchema createResponse

// CreateSuccessSchema is a schema-only success envelope for checkout-decision create.
type CreateSuccessSchema struct {
	Data CreateResponseSchema            `json:"data"`
	Meta serdser.SuccessMetaNoPagination `json:"meta"`
}
