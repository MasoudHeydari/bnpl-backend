package serdser

import (
	"time"

	"github.com/gin-gonic/gin"
	corepagination "github.com/parstis/bnpl-backend/pkg/core/pagination"
	"github.com/parstis/bnpl-backend/pkg/core/trace"
)

// SuccessCode identifies a stable successful response category.
//
// Success codes are machine-readable identifiers intended for clients,
// frontend applications, monitoring, and API analytics.
//
// Success codes should be stable once published. Changing an existing success
// code is considered a breaking API contract change.
type SuccessCode string

const (
	// SuccessCodeDecisionCreated indicates that a decision was created.
	SuccessCodeDecisionCreated SuccessCode = "DECISION_CREATED"

	// SuccessCodeDecisionExpired indicates that a decision was expired.
	SuccessCodeDecisionExpired SuccessCode = "DECISION_EXPIRED"

	// SuccessCodeDecisionOverridden indicates that a decision was overridden.
	SuccessCodeDecisionOverridden SuccessCode = "DECISION_OVERRIDDEN"

	// SuccessCodeContractCreated indicates that a contract was created.
	SuccessCodeContractCreated SuccessCode = "CONTRACT_CREATED"

	// SuccessCodeRepaymentPlanCreated indicates that a repayment plan was created.
	SuccessCodeRepaymentPlanCreated SuccessCode = "REPAYMENT_PLAN_CREATED"

	// SuccessCodeInstallmentsListed indicates that installments were listed.
	SuccessCodeInstallmentsListed SuccessCode = "INSTALLMENTS_LISTED"

	// SuccessCodePaymentIntentCreated indicates that a payment intent was created.
	SuccessCodePaymentIntentCreated SuccessCode = "PAYMENT_INTENT_CREATED"

	// SuccessCodeHealthOK indicates that a health check completed successfully.
	SuccessCodeHealthOK SuccessCode = "HEALTH_OK"
)

// SuccessResponse is the platform-wide successful response envelope.
//
// Every 2xx REST response should use this structure.
//
// Data contains the endpoint-specific response payload. It may be nil, in which
// case it is serialized as JSON null.
//
// Meta contains standard response metadata such as status, stable response code,
// HTTP status code, timestamps, trace identifiers, and optional pagination.
type SuccessResponse struct {
	Data any         `json:"data"`
	Meta SuccessMeta `json:"meta"`
}

// SuccessMeta carries standard metadata for successful REST responses.
//
// Status is always "success" for successful responses.
//
// Code is a stable machine-readable success identifier.
//
// Message is human-readable and may change over time. Clients should not use it
// as a stable contract.
//
// StatusCode mirrors the HTTP response status code for clients that process the
// body independently of the HTTP layer.
//
// Timestamp is generated in UTC using RFC3339 format.
//
// TraceID, RequestID, and CorrelationID are used for observability and request
// correlation.
//
// Pagination is populated only for list endpoints.
type SuccessMeta struct {
	Status        string          `json:"status"`
	Code          string          `json:"code"`
	Message       string          `json:"message"`
	StatusCode    int             `json:"status_code"`
	Timestamp     string          `json:"timestamp"`
	TraceID       string          `json:"trace_id"`
	RequestID     string          `json:"request_id"`
	CorrelationID *string         `json:"correlation_id,omitempty"`
	Pagination    *PaginationMeta `json:"pagination,omitempty"`
}

// PaginationMeta describes pagination state for list responses.
//
// Exactly one of Page, Offset, Cursor, or Keyset should be populated according
// to Type.
type PaginationMeta struct {
	Type        string      `json:"type"`
	Returned    int         `json:"returned"`
	HasNext     bool        `json:"has_next"`
	HasPrevious bool        `json:"has_previous"`
	Page        *PageInfo   `json:"page,omitempty"`
	Offset      *OffsetInfo `json:"offset,omitempty"`
	Cursor      *CursorInfo `json:"cursor,omitempty"`
	Keyset      *KeysetInfo `json:"keyset,omitempty"`
}

// PageInfo is populated when PaginationMeta.Type is "page".
type PageInfo struct {
	CurrentPage int   `json:"current_page"`
	PageSize    int   `json:"page_size"`
	TotalItems  int64 `json:"total_items"`
	TotalPages  int   `json:"total_pages"`
}

// OffsetInfo is populated when PaginationMeta.Type is "offset".
type OffsetInfo struct {
	Offset     int    `json:"offset"`
	Limit      int    `json:"limit"`
	TotalItems *int64 `json:"total_items,omitempty"`
}

// CursorInfo is populated when PaginationMeta.Type is "cursor".
type CursorInfo struct {
	Limit          int     `json:"limit"`
	NextCursor     *string `json:"next_cursor,omitempty"`
	PreviousCursor *string `json:"previous_cursor,omitempty"`
}

// KeysetInfo is populated when PaginationMeta.Type is "keyset".
type KeysetInfo struct {
	Limit              int     `json:"limit"`
	NextAfterID        *string `json:"next_after_id,omitempty"`
	NextAfterCreatedAt *string `json:"next_after_created_at,omitempty"`
}

// buildMeta constructs SuccessMeta from the request context.
func buildMeta(
	c *gin.Context,
	statusCode int,
	code SuccessCode,
	message string,
	pagination *PaginationMeta,
) SuccessMeta {
	ctx := c.Request.Context()

	var correlationID *string
	if cid := trace.CorrelationID(ctx); cid != "" {
		correlationID = &cid
	}

	return SuccessMeta{
		Status:        "success",
		Code:          string(code),
		Message:       message,
		StatusCode:    statusCode,
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		TraceID:       trace.TraceID(ctx),
		RequestID:     trace.RequestID(ctx),
		CorrelationID: correlationID,
		Pagination:    pagination,
	}
}

// Ser writes a non-paginated successful REST response.
func Ser(c *gin.Context, statusCode int, code SuccessCode, message string, data any) {
	c.JSON(statusCode, SuccessResponse{
		Data: data,
		Meta: buildMeta(c, statusCode, code, message, nil),
	})
}

// SerPage writes a page-number paginated successful REST response.
func SerPage[T any](
	c *gin.Context,
	statusCode int,
	code SuccessCode,
	message string,
	page corepagination.OffsetPage[T],
) {
	m := page.Meta

	pagination := &PaginationMeta{
		Type:        "page",
		Returned:    len(page.Data),
		HasNext:     m.HasNext,
		HasPrevious: m.HasPrevious,
		Page: &PageInfo{
			CurrentPage: m.Page,
			PageSize:    m.Limit,
			TotalItems:  m.Total,
			TotalPages:  m.TotalPages,
		},
	}

	c.JSON(statusCode, SuccessResponse{
		Data: page.Data,
		Meta: buildMeta(c, statusCode, code, message, pagination),
	})
}

// SerCursor writes a cursor-paginated successful REST response.
func SerCursor[T any](
	c *gin.Context,
	statusCode int,
	code SuccessCode,
	message string,
	page corepagination.CursorPage[T],
) {
	m := page.Meta

	pagination := &PaginationMeta{
		Type:     "cursor",
		Returned: len(page.Data),
		HasNext:  m.HasNext,
		Cursor: &CursorInfo{
			Limit:      m.Limit,
			NextCursor: m.NextCursor,
		},
	}

	c.JSON(statusCode, SuccessResponse{
		Data: page.Data,
		Meta: buildMeta(c, statusCode, code, message, pagination),
	})
}

// SerKeyset writes a keyset-paginated successful REST response.
func SerKeyset[T any](
	c *gin.Context,
	statusCode int,
	code SuccessCode,
	message string,
	page corepagination.KeysetPage[T],
) {
	m := page.Meta

	pagination := &PaginationMeta{
		Type:     "keyset",
		Returned: len(page.Data),
		HasNext:  m.HasNext,
		Keyset: &KeysetInfo{
			Limit:              m.Limit,
			NextAfterID:        m.NextAfterID,
			NextAfterCreatedAt: m.NextAfterCreatedAt,
		},
	}

	c.JSON(statusCode, SuccessResponse{
		Data: page.Data,
		Meta: buildMeta(c, statusCode, code, message, pagination),
	})
}
