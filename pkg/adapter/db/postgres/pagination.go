package postgres

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/parstis/bnpl-backend/pkg/core/pagination"
)

// Pagination SQL helper errors.
var (
	ErrInvalidSortField = errors.New("invalid sort field")
	ErrUnsafeSQLColumn  = errors.New("unsafe sql column")
)

var safeColumnPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*(\.[a-zA-Z_][a-zA-Z0-9_]*)?$`)

// SortMap maps API sort fields to safe SQL column identifiers.
type SortMap map[pagination.SortField]string

// Column returns the SQL column for field or an error when the field is unknown or unsafe.
func (m SortMap) Column(field pagination.SortField) (string, error) {
	const op = "postgres.SortMap.Column"

	column, ok := m[field]
	if !ok {
		return "", fmt.Errorf("%s: %w: %s", op, ErrInvalidSortField, field)
	}

	if !isSafeColumn(column) {
		return "", fmt.Errorf("%s: %w: %s", op, ErrUnsafeSQLColumn, column)
	}

	return column, nil
}

// OrderBy builds an ORDER BY clause from sortMap, sort, order, and optional tie-breaker columns.
func OrderBy(sortMap SortMap, sort pagination.SortField, order pagination.Order, tieBreakers ...string) (string, error) {
	const op = "postgres.OrderBy"

	column, err := sortMap.Column(sort)
	if err != nil {
		return "", err
	}

	if !order.IsValid() {
		order = pagination.DefaultOrder
	}

	parts := []string{column + " " + order.SQL()}

	for _, tieBreaker := range tieBreakers {
		if !isSafeColumn(tieBreaker) {
			return "", fmt.Errorf("%s: %w: %s", op, ErrUnsafeSQLColumn, tieBreaker)
		}

		parts = append(parts, tieBreaker+" "+order.SQL())
	}

	return "ORDER BY " + strings.Join(parts, ", "), nil
}

// LimitOffset builds LIMIT/OFFSET SQL fragments and args for an offset pagination request.
func LimitOffset(req pagination.OffsetRequest, placeholderStart int) (string, []any) {
	req.Normalize()

	return fmt.Sprintf(
			"LIMIT $%d OFFSET $%d",
			placeholderStart,
			placeholderStart+1,
		), []any{
			req.Limit,
			req.SQLOffset(),
		}
}

// KeysetWhere builds a keyset pagination WHERE clause and args when an after cursor is present.
//
// timestampColumn is the seek timestamp column (for example created_at or due_at).
func KeysetWhere(
	timestampColumn string,
	idColumn string,
	req pagination.KeysetRequest,
	placeholderStart int,
) (string, []any, error) {
	const op = "postgres.KeysetWhere"

	req.Normalize()

	if !isSafeColumn(timestampColumn) {
		return "", nil, fmt.Errorf("%s: %w: %s", op, ErrUnsafeSQLColumn, timestampColumn)
	}

	if !isSafeColumn(idColumn) {
		return "", nil, fmt.Errorf("%s: %w: %s", op, ErrUnsafeSQLColumn, idColumn)
	}

	if err := req.Validate(); err != nil {
		return "", nil, err
	}

	if !req.HasAfter() {
		return "", nil, nil
	}

	afterID, err := uuid.Parse(req.AfterID)
	if err != nil {
		return "", nil, fmt.Errorf("%s: %w: invalid after_id: %w", op, pagination.ErrInvalidKeyset, err)
	}

	afterTimestamp, err := time.Parse(time.RFC3339Nano, req.AfterTimestamp())
	if err != nil {
		return "", nil, fmt.Errorf("%s: %w: invalid after timestamp: %w", op, pagination.ErrInvalidKeyset, err)
	}

	operator := "<"
	if req.Order == pagination.OrderAsc {
		operator = ">"
	}

	where := fmt.Sprintf(
		"(%s, %s) %s ($%d, $%d)",
		timestampColumn,
		idColumn,
		operator,
		placeholderStart,
		placeholderStart+1,
	)

	return where, []any{afterTimestamp, afterID}, nil
}

// CursorWhere builds a cursor pagination WHERE clause comparing (sortColumn, idColumn) tuples.
func CursorWhere(
	sortColumn string,
	idColumn string,
	sortValue any,
	id uuid.UUID,
	order pagination.Order,
	placeholderStart int,
) (string, []any, error) {
	const op = "postgres.CursorWhere"

	if !isSafeColumn(sortColumn) {
		return "", nil, fmt.Errorf("%s: %w: %s", op, ErrUnsafeSQLColumn, sortColumn)
	}

	if !isSafeColumn(idColumn) {
		return "", nil, fmt.Errorf("%s: %w: %s", op, ErrUnsafeSQLColumn, idColumn)
	}

	if id == uuid.Nil {
		return "", nil, fmt.Errorf("%s: %w: missing cursor id", op, pagination.ErrInvalidCursor)
	}

	if !order.IsValid() {
		order = pagination.DefaultOrder
	}

	operator := "<"
	if order == pagination.OrderAsc {
		operator = ">"
	}

	where := fmt.Sprintf(
		"(%s, %s) %s ($%d, $%d)",
		sortColumn,
		idColumn,
		operator,
		placeholderStart,
		placeholderStart+1,
	)

	return where, []any{sortValue, id}, nil
}

// Limit builds a LIMIT SQL fragment clamped to pagination defaults and maximums.
func Limit(reqLimit int, placeholderStart int) (string, []any) {
	if reqLimit < 1 {
		reqLimit = pagination.DefaultLimit
	}

	if reqLimit > pagination.MaxLimit {
		reqLimit = pagination.MaxLimit
	}

	return fmt.Sprintf("LIMIT $%d", placeholderStart), []any{reqLimit}
}

func isSafeColumn(column string) bool {
	column = strings.TrimSpace(column)
	if column == "" {
		return false
	}

	return safeColumnPattern.MatchString(column)
}
