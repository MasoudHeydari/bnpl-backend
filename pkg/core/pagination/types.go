package pagination

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	// DefaultLimit is the page size used when a request omits limit.
	DefaultLimit = 1

	// MaxLimit is the maximum allowed page size after normalization.
	MaxLimit = 100

	// DefaultPage is the page number used when a request omits page.
	DefaultPage = 1

	// DefaultSort is the logical sort field used when a request omits sort.
	DefaultSort = SortField("created_at")

	// DefaultOrder is the sort direction used when a request omits order.
	DefaultOrder = OrderDesc

	cursorVersion = 1
)

var (
	// ErrInvalidCursor is returned when an opaque cursor token cannot be decoded
	// or fails validation.
	ErrInvalidCursor = errors.New("invalid cursor")

	// ErrInvalidKeyset is returned when a keyset pagination request is malformed.
	ErrInvalidKeyset = errors.New("invalid keyset pagination request")
)

// Order defines pagination sort direction.
type Order string

const (
	// OrderAsc sorts results in ascending order.
	OrderAsc Order = "asc"

	// OrderDesc sorts results in descending order.
	OrderDesc Order = "desc"
)

// IsValid reports whether o is a supported sort order.
func (o Order) IsValid() bool {
	return o == OrderAsc || o == OrderDesc
}

// SQL returns the SQL keyword for the order.
//
// The returned value is safe because Order is restricted to ASC/DESC.
func (o Order) SQL() string {
	if o == OrderAsc {
		return "ASC"
	}

	return "DESC"
}

// SortField is a logical field name used for sorting.
//
// SortField must be mapped to a concrete database column by the repository
// through a whitelist. It must never be used directly in SQL.
type SortField string

// String returns the raw logical sort field.
func (s SortField) String() string {
	return string(s)
}

// OffsetRequest is the input for page-number pagination.
type OffsetRequest struct {
	// Page is the 1-based page number to return.
	Page int

	// Limit is the maximum number of items per page.
	Limit int

	// Sort is the logical field used for ordering.
	Sort SortField

	// Order is the sort direction.
	Order Order
}

// Normalize applies safe defaults and caps.
func (r *OffsetRequest) Normalize() {
	if r.Page < 1 {
		r.Page = DefaultPage
	}

	if r.Limit < 1 {
		r.Limit = DefaultLimit
	}

	if r.Limit > MaxLimit {
		r.Limit = MaxLimit
	}

	if r.Sort == "" {
		r.Sort = DefaultSort
	}

	if !r.Order.IsValid() {
		r.Order = DefaultOrder
	}
}

// SQLOffset returns the SQL OFFSET value for this request.
//
// Invalid page or limit values return zero. Results that would overflow int
// saturate to math.MaxInt.
func (r OffsetRequest) SQLOffset() int {
	if r.Page < 1 || r.Limit < 1 {
		return 0
	}

	pageOffset := int64(r.Page - 1)
	limit := int64(r.Limit)
	if pageOffset > 0 && limit > math.MaxInt/pageOffset {
		return math.MaxInt
	}

	return int(pageOffset * limit)
}

// CursorRequest is the input for opaque cursor pagination.
type CursorRequest struct {
	// Cursor is an opaque token positioning the next page; empty starts from the beginning.
	Cursor string

	// Limit is the maximum number of items to return.
	Limit int

	// Sort is the logical field used for ordering.
	Sort SortField

	// Order is the sort direction.
	Order Order
}

// Normalize applies safe defaults and caps.
func (r *CursorRequest) Normalize() {
	if r.Limit < 1 {
		r.Limit = DefaultLimit
	}

	if r.Limit > MaxLimit {
		r.Limit = MaxLimit
	}

	if r.Sort == "" {
		r.Sort = DefaultSort
	}

	if !r.Order.IsValid() {
		r.Order = DefaultOrder
	}
}

// KeysetRequest is the input for seek pagination ordered by a timestamp column
// and id.
//
// Provide either AfterCreatedAt or AfterDueAt together with AfterID. Mixing both
// timestamp cursors is invalid.
type KeysetRequest struct {
	// AfterID is the id of the last item from the previous page.
	AfterID string

	// AfterCreatedAt is the created_at seek cursor from the previous page.
	AfterCreatedAt string

	// AfterDueAt is the due_at seek cursor from the previous page.
	AfterDueAt string

	// Limit is the maximum number of items to return.
	Limit int

	// Order is the sort direction for the seek query.
	Order Order
}

// Normalize applies safe defaults and caps.
func (r *KeysetRequest) Normalize() {
	if r.Limit < 1 {
		r.Limit = DefaultLimit
	}

	if r.Limit > MaxLimit {
		r.Limit = MaxLimit
	}

	if !r.Order.IsValid() {
		r.Order = DefaultOrder
	}
}

// HasAfter reports whether the request contains a complete seek position.
func (r KeysetRequest) HasAfter() bool {
	return r.AfterID != "" && r.AfterTimestamp() != ""
}

// AfterTimestamp returns the seek timestamp cursor value.
//
// Prefer AfterDueAt when set; otherwise AfterCreatedAt.
func (r KeysetRequest) AfterTimestamp() string {
	if r.AfterDueAt != "" {
		return r.AfterDueAt
	}

	return r.AfterCreatedAt
}

// Validate validates the seek position when one is provided.
func (r KeysetRequest) Validate() error {
	const op = "pagination.KeysetRequest.Validate"

	if r.AfterID == "" && r.AfterCreatedAt == "" && r.AfterDueAt == "" {
		return nil
	}

	if r.AfterCreatedAt != "" && r.AfterDueAt != "" {
		return fmt.Errorf("%s: %w: after_created_at and after_due_at cannot both be set", op, ErrInvalidKeyset)
	}

	if r.AfterID == "" || r.AfterTimestamp() == "" {
		return fmt.Errorf("%s: %w: after_id and after timestamp must be provided together", op, ErrInvalidKeyset)
	}

	if _, err := uuid.Parse(r.AfterID); err != nil {
		return fmt.Errorf("%s: %w: invalid after_id: %w", op, ErrInvalidKeyset, err)
	}

	if _, err := time.Parse(time.RFC3339Nano, r.AfterTimestamp()); err != nil {
		return fmt.Errorf("%s: %w: invalid after timestamp: %w", op, ErrInvalidKeyset, err)
	}

	return nil
}

// PageMeta describes page-number pagination results.
type PageMeta struct {
	// PaginationType identifies the pagination strategy (for example "page").
	PaginationType string `json:"pagination_type"`

	// Page is the current 1-based page number.
	Page int `json:"page"`

	// Limit is the page size used for this response.
	Limit int `json:"limit"`

	// Total is the total number of matching items.
	Total int64 `json:"total"`

	// TotalPages is the number of pages implied by Total and Limit.
	TotalPages int `json:"total_pages"`

	// HasNext reports whether a subsequent page exists.
	HasNext bool `json:"has_next"`

	// HasPrevious reports whether a previous page exists.
	HasPrevious bool `json:"has_previous"`
}

// NewPageMeta builds PageMeta from a normalized OffsetRequest and total count.
func NewPageMeta(req OffsetRequest, total int64) PageMeta {
	if req.Limit <= 0 {
		return PageMeta{
			PaginationType: "page",
			Page:           req.Page,
			Limit:          req.Limit,
			Total:          total,
			TotalPages:     0,
			HasNext:        false,
			HasPrevious:    false,
		}
	}

	totalPages := int((total + int64(req.Limit) - 1) / int64(req.Limit))

	return PageMeta{
		PaginationType: "page",
		Page:           req.Page,
		Limit:          req.Limit,
		Total:          total,
		TotalPages:     totalPages,
		HasNext:        int64(req.Page*req.Limit) < total,
		HasPrevious:    req.Page > 1,
	}
}

// CursorMeta describes cursor pagination results.
type CursorMeta struct {
	// PaginationType identifies the pagination strategy (for example "cursor").
	PaginationType string `json:"pagination_type"`

	// Limit is the page size used for this response.
	Limit int `json:"limit"`

	// NextCursor is the opaque token for the next page, when HasNext is true.
	NextCursor *string `json:"next_cursor"`

	// HasNext reports whether a subsequent page exists.
	HasNext bool `json:"has_next"`
}

// KeysetMeta describes keyset pagination results.
type KeysetMeta struct {
	// PaginationType identifies the pagination strategy (for example "keyset").
	PaginationType string `json:"pagination_type"`

	// Limit is the page size used for this response.
	Limit int `json:"limit"`

	// NextAfterID is the id seek cursor for the next page, when HasNext is true.
	NextAfterID *string `json:"next_after_id"`

	// NextAfterCreatedAt is the created_at seek cursor for the next page.
	NextAfterCreatedAt *string `json:"next_after_created_at,omitempty"`

	// NextAfterDueAt is the due_at seek cursor for the next page.
	NextAfterDueAt *string `json:"next_after_due_at,omitempty"`

	// HasNext reports whether a subsequent page exists.
	HasNext bool `json:"has_next"`
}

// OffsetPage is a page-number paginated result.
type OffsetPage[T any] struct {
	// Data contains the items for the current page.
	Data []T `json:"data"`

	// Meta describes page-number pagination for this response.
	Meta PageMeta `json:"meta"`
}

// CursorPage is an opaque-cursor paginated result.
type CursorPage[T any] struct {
	// Data contains the items for the current page.
	Data []T `json:"data"`

	// Meta describes cursor pagination for this response.
	Meta CursorMeta `json:"meta"`
}

// KeysetPage is a seek-paginated result.
type KeysetPage[T any] struct {
	// Data contains the items for the current page.
	Data []T `json:"data"`

	// Meta describes keyset pagination for this response.
	Meta KeysetMeta `json:"meta"`
}

// Cursor is the decoded form of an opaque pagination cursor token.
type Cursor struct {
	// Version is the opaque cursor payload format version.
	Version int `json:"ver"`

	// SortVal is the sort-column value at the cursor position.
	SortVal string `json:"v"`

	// ID is the row identifier at the cursor position.
	ID uuid.UUID `json:"id"`

	// Sort is the logical sort field encoded with the cursor.
	Sort SortField `json:"s,omitempty"`

	// Order is the sort direction encoded with the cursor.
	Order Order `json:"o,omitempty"`
}

// NewCursor creates a versioned cursor.
func NewCursor(sortVal string, id uuid.UUID, sort SortField, order Order) Cursor {
	if sort == "" {
		sort = DefaultSort
	}

	if !order.IsValid() {
		order = DefaultOrder
	}

	return Cursor{
		Version: cursorVersion,
		SortVal: sortVal,
		ID:      id,
		Sort:    sort,
		Order:   order,
	}
}

// Encode returns a URL-safe opaque cursor token.
func (c Cursor) Encode() (string, error) {
	const op = "pagination.Cursor.Encode"

	if c.Version == 0 {
		c.Version = cursorVersion
	}

	if err := c.Validate(); err != nil {
		return "", err
	}

	b, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("%s: encode cursor: %w", op, err)
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Validate validates a decoded cursor.
func (c Cursor) Validate() error {
	const op = "pagination.Cursor.Validate"

	if c.Version != cursorVersion {
		return fmt.Errorf("%s: %w: unsupported cursor version %d", op, ErrInvalidCursor, c.Version)
	}

	if c.SortVal == "" {
		return fmt.Errorf("%s: %w: missing sort value", op, ErrInvalidCursor)
	}

	if c.ID == uuid.Nil {
		return fmt.Errorf("%s: %w: missing id", op, ErrInvalidCursor)
	}

	if c.Sort == "" {
		return fmt.Errorf("%s: %w: missing sort field", op, ErrInvalidCursor)
	}

	if !c.Order.IsValid() {
		return fmt.Errorf("%s: %w: invalid order", op, ErrInvalidCursor)
	}

	return nil
}

// DecodeCursor parses a cursor token produced by Cursor.Encode.
//
// An empty token returns a zero Cursor without error and represents the first
// page.
func DecodeCursor(token string) (Cursor, error) {
	const op = "pagination.DecodeCursor"

	token = strings.TrimSpace(token)
	if token == "" {
		return Cursor{}, nil
	}

	b, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return Cursor{}, fmt.Errorf("%s: %w: invalid base64: %w", op, ErrInvalidCursor, err)
	}

	var c Cursor
	if err = json.Unmarshal(b, &c); err != nil {
		return Cursor{}, fmt.Errorf("%s: %w: invalid json: %w", op, ErrInvalidCursor, err)
	}

	if err = c.Validate(); err != nil {
		return Cursor{}, err
	}

	return c, nil
}
