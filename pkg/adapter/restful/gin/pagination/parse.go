package pagination

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	corepagination "github.com/parstis/bnpl-backend/pkg/core/pagination"
)

// ParseOffset reads page-number pagination query parameters.
//
// Query params:
//
//   - page
//   - limit
//   - sort
//   - order
func ParseOffset(c *gin.Context) (corepagination.OffsetRequest, bool) {
	req := corepagination.OffsetRequest{
		Sort:  corepagination.SortField(strings.TrimSpace(c.Query("sort"))),
		Order: corepagination.Order(strings.TrimSpace(c.Query("order"))),
	}

	page, ok := parsePositiveIntParam(c, "page")
	if !ok {
		return corepagination.OffsetRequest{}, false
	}
	if page != nil {
		req.Page = *page
	}

	limit, ok := parsePositiveIntParam(c, "limit")
	if !ok {
		return corepagination.OffsetRequest{}, false
	}
	if limit != nil {
		req.Limit = *limit
	}

	if !parseOrder(c, req.Order) {
		return corepagination.OffsetRequest{}, false
	}

	req.Normalize()
	return req, true
}

// ParseCursor reads opaque cursor pagination query parameters.
//
// Query params:
//
//   - cursor
//   - limit
//   - sort
//   - order
func ParseCursor(c *gin.Context) (corepagination.CursorRequest, bool) {
	cursor := strings.TrimSpace(c.Query("cursor"))

	req := corepagination.CursorRequest{
		Cursor: cursor,
		Sort:   corepagination.SortField(strings.TrimSpace(c.Query("sort"))),
		Order:  corepagination.Order(strings.TrimSpace(c.Query("order"))),
	}

	limit, ok := parsePositiveIntParam(c, "limit")
	if !ok {
		return corepagination.CursorRequest{}, false
	}
	if limit != nil {
		req.Limit = *limit
	}

	if !parseOrder(c, req.Order) {
		return corepagination.CursorRequest{}, false
	}

	if cursor != "" {
		if _, err := corepagination.DecodeCursor(cursor); err != nil {
			serdser.SerValidationErr(c, "cursor", "cursor is invalid")
			return corepagination.CursorRequest{}, false
		}
	}

	req.Normalize()
	return req, true
}

// ParseKeyset reads keyset pagination query parameters.
//
// Query params:
//
//   - after_id
//   - after_created_at
//   - limit
//   - order
func ParseKeyset(c *gin.Context) (corepagination.KeysetRequest, bool) {
	req := corepagination.KeysetRequest{
		AfterID:        strings.TrimSpace(c.Query("after_id")),
		AfterCreatedAt: strings.TrimSpace(c.Query("after_created_at")),
		Order:          corepagination.Order(strings.TrimSpace(c.Query("order"))),
	}

	limit, ok := parsePositiveIntParam(c, "limit")
	if !ok {
		return corepagination.KeysetRequest{}, false
	}
	if limit != nil {
		req.Limit = *limit
	}

	if !parseOrder(c, req.Order) {
		return corepagination.KeysetRequest{}, false
	}

	req.Normalize()

	if err := req.Validate(); err != nil {
		serdser.SerValidationErr(c, "after", "after_id and after_created_at must form a valid keyset cursor")
		return corepagination.KeysetRequest{}, false
	}

	return req, true
}

// parsePositiveIntParam parses a positive integer query parameter.
//
// A missing parameter returns nil, true.
func parsePositiveIntParam(c *gin.Context, name string) (*int, bool) {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return nil, true
	}

	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		serdser.SerValidationErr(c, name, name+" must be a positive integer")
		return nil, false
	}

	return &n, true
}

// parseOrder validates an optional order query parameter.
//
// Empty order is valid and will be defaulted by core pagination normalization.
func parseOrder(c *gin.Context, order corepagination.Order) bool {
	if order == "" || order.IsValid() {
		return true
	}

	serdser.SerValidationErr(c, "order", "order must be either asc or desc")
	return false
}
