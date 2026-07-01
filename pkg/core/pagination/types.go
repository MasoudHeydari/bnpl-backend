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
	DefaultLimit = 20
	MaxLimit     = 100
	DefaultPage  = 1
	DefaultSort  = SortField("created_at")
	DefaultOrder = OrderDesc

	cursorVersion = 1
)

var (
	ErrInvalidCursor = errors.New("invalid cursor")
	ErrInvalidKeyset = errors.New("invalid keyset pagination request")
)

// Order defines pagination sort direction.
type Order string

const (
	OrderAsc  Order = "asc"
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
	Page  int
	Limit int
	Sort  SortField
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
	Cursor string
	Limit  int
	Sort   SortField
	Order  Order
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

// KeysetRequest is the input for seek pagination ordered by created_at and id.
type KeysetRequest struct {
	AfterID        string
	AfterCreatedAt string
	Limit          int
	Order          Order
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
	return r.AfterID != "" && r.AfterCreatedAt != ""
}

// Validate validates the seek position when one is provided.
func (r KeysetRequest) Validate() error {
	if r.AfterID == "" && r.AfterCreatedAt == "" {
		return nil
	}
	if r.AfterID == "" || r.AfterCreatedAt == "" {
		return fmt.Errorf("%w: after_id and after_created_at must be provided together", ErrInvalidKeyset)
	}
	if _, err := uuid.Parse(r.AfterID); err != nil {
		return fmt.Errorf("%w: invalid after_id: %w", ErrInvalidKeyset, err)
	}
	if _, err := time.Parse(time.RFC3339Nano, r.AfterCreatedAt); err != nil {
		return fmt.Errorf("%w: invalid after_created_at: %w", ErrInvalidKeyset, err)
	}
	return nil
}

// PageMeta describes page-number pagination results.
type PageMeta struct {
	PaginationType string `json:"pagination_type"`
	Page           int    `json:"page"`
	Limit          int    `json:"limit"`
	Total          int64  `json:"total"`
	TotalPages     int    `json:"total_pages"`
	HasNext        bool   `json:"has_next"`
	HasPrevious    bool   `json:"has_previous"`
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
	PaginationType string  `json:"pagination_type"`
	Limit          int     `json:"limit"`
	NextCursor     *string `json:"next_cursor"`
	HasNext        bool    `json:"has_next"`
}

// KeysetMeta describes keyset pagination results.
type KeysetMeta struct {
	PaginationType     string  `json:"pagination_type"`
	Limit              int     `json:"limit"`
	NextAfterID        *string `json:"next_after_id"`
	NextAfterCreatedAt *string `json:"next_after_created_at"`
	HasNext            bool    `json:"has_next"`
}

// OffsetPage is a page-number paginated result.
type OffsetPage[T any] struct {
	Data []T      `json:"data"`
	Meta PageMeta `json:"meta"`
}

// CursorPage is an opaque-cursor paginated result.
type CursorPage[T any] struct {
	Data []T        `json:"data"`
	Meta CursorMeta `json:"meta"`
}

// KeysetPage is a seek-paginated result.
type KeysetPage[T any] struct {
	Data []T        `json:"data"`
	Meta KeysetMeta `json:"meta"`
}

// Cursor is the decoded form of an opaque pagination cursor token.
type Cursor struct {
	Version int       `json:"ver"`
	SortVal string    `json:"v"`
	ID      uuid.UUID `json:"id"`
	Sort    SortField `json:"s,omitempty"`
	Order   Order     `json:"o,omitempty"`
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
	if c.Version == 0 {
		c.Version = cursorVersion
	}
	if err := c.Validate(); err != nil {
		return "", err
	}

	b, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("encode cursor: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Validate validates a decoded cursor.
func (c Cursor) Validate() error {
	if c.Version != cursorVersion {
		return fmt.Errorf("%w: unsupported cursor version %d", ErrInvalidCursor, c.Version)
	}
	if c.SortVal == "" {
		return fmt.Errorf("%w: missing sort value", ErrInvalidCursor)
	}
	if c.ID == uuid.Nil {
		return fmt.Errorf("%w: missing id", ErrInvalidCursor)
	}
	if c.Sort == "" {
		return fmt.Errorf("%w: missing sort field", ErrInvalidCursor)
	}
	if !c.Order.IsValid() {
		return fmt.Errorf("%w: invalid order", ErrInvalidCursor)
	}
	return nil
}

// DecodeCursor parses a cursor token produced by Cursor.Encode.
//
// An empty token returns a zero Cursor without error and represents the first
// page.
func DecodeCursor(token string) (Cursor, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return Cursor{}, nil
	}

	b, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return Cursor{}, fmt.Errorf("%w: invalid base64: %w", ErrInvalidCursor, err)
	}

	var c Cursor
	if err = json.Unmarshal(b, &c); err != nil {
		return Cursor{}, fmt.Errorf("%w: invalid json: %w", ErrInvalidCursor, err)
	}
	if err = c.Validate(); err != nil {
		return Cursor{}, err
	}

	return c, nil
}
