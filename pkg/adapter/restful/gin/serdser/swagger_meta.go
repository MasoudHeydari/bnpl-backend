package serdser

// SuccessMetaNoPagination is a swagger-only mirror of SuccessMeta for non-list
// endpoints. Runtime responses still use SuccessMeta; pagination is omitted on
// the wire when unset, and these schemas keep OpenAPI Example Value aligned.
type SuccessMetaNoPagination struct {
	Status        string  `json:"status" example:"success" enums:"success" i18n:"canonical"`
	Code          string  `json:"code" example:"DECISION_CREATED" i18n:"canonical"`
	Message       string  `json:"message" example:"Checkout decision created." i18n:"localized"`
	StatusCode    int     `json:"status_code" example:"201"`
	Timestamp     string  `json:"timestamp" example:"2026-08-09T12:00:00Z"`
	TraceID       string  `json:"trace_id" example:"a1b2c3d4-e5f6-7890-abcd-ef1234567890" format:"uuid"`
	RequestID     string  `json:"request_id" example:"b2c3d4e5-f6a7-8901-bcde-f12345678901" format:"uuid"`
	CorrelationID *string `json:"correlation_id,omitempty" example:"c3d4e5f6-a7b8-9012-cdef-123456789012" format:"uuid"`
}

// SuccessMetaKeyset is a swagger-only success meta for keyset-paginated lists.
// It documents only the keyset pagination shape these endpoints return.
type SuccessMetaKeyset struct {
	Status        string                `json:"status" example:"success" enums:"success" i18n:"canonical"`
	Code          string                `json:"code" example:"ACTIVE_CONTRACTS_LISTED" i18n:"canonical"`
	Message       string                `json:"message" example:"Current contracts listed." i18n:"localized"`
	StatusCode    int                   `json:"status_code" example:"200"`
	Timestamp     string                `json:"timestamp" example:"2026-08-09T12:00:00Z"`
	TraceID       string                `json:"trace_id" example:"a1b2c3d4-e5f6-7890-abcd-ef1234567890" format:"uuid"`
	RequestID     string                `json:"request_id" example:"b2c3d4e5-f6a7-8901-bcde-f12345678901" format:"uuid"`
	CorrelationID *string               `json:"correlation_id,omitempty" example:"c3d4e5f6-a7b8-9012-cdef-123456789012" format:"uuid"`
	Pagination    *KeysetPaginationMeta `json:"pagination"`
}

// KeysetPaginationMeta is the swagger-facing pagination block for keyset lists.
type KeysetPaginationMeta struct {
	Type        string      `json:"type" example:"keyset" enums:"keyset"`
	Returned    int         `json:"returned" example:"2"`
	HasNext     bool        `json:"has_next" example:"false"`
	HasPrevious bool        `json:"has_previous" example:"false"`
	Keyset      *KeysetInfo `json:"keyset,omitempty"`
}
