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

var (
	ErrInvalidSortField = errors.New("invalid sort field")
	ErrUnsafeSQLColumn  = errors.New("unsafe sql column")
)

var safeColumnPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*(\.[a-zA-Z_][a-zA-Z0-9_]*)?$`)

type SortMap map[pagination.SortField]string

func (m SortMap) Column(field pagination.SortField) (string, error) {
	column, ok := m[field]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrInvalidSortField, field)
	}
	if !isSafeColumn(column) {
		return "", fmt.Errorf("%w: %s", ErrUnsafeSQLColumn, column)
	}
	return column, nil
}

func OrderBy(sortMap SortMap, sort pagination.SortField, order pagination.Order, tieBreakers ...string) (string, error) {
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
			return "", fmt.Errorf("%w: %s", ErrUnsafeSQLColumn, tieBreaker)
		}
		parts = append(parts, tieBreaker+" "+order.SQL())
	}

	return "ORDER BY " + strings.Join(parts, ", "), nil
}

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

func KeysetWhere(
	createdAtColumn string,
	idColumn string,
	req pagination.KeysetRequest,
	placeholderStart int,
) (string, []any, error) {
	req.Normalize()

	if !isSafeColumn(createdAtColumn) {
		return "", nil, fmt.Errorf("%w: %s", ErrUnsafeSQLColumn, createdAtColumn)
	}
	if !isSafeColumn(idColumn) {
		return "", nil, fmt.Errorf("%w: %s", ErrUnsafeSQLColumn, idColumn)
	}

	if err := req.Validate(); err != nil {
		return "", nil, err
	}
	if !req.HasAfter() {
		return "", nil, nil
	}

	afterID, err := uuid.Parse(req.AfterID)
	if err != nil {
		return "", nil, fmt.Errorf("%w: invalid after_id: %w", pagination.ErrInvalidKeyset, err)
	}

	afterCreatedAt, err := time.Parse(time.RFC3339Nano, req.AfterCreatedAt)
	if err != nil {
		return "", nil, fmt.Errorf("%w: invalid after_created_at: %w", pagination.ErrInvalidKeyset, err)
	}

	operator := "<"
	if req.Order == pagination.OrderAsc {
		operator = ">"
	}

	where := fmt.Sprintf(
		"(%s, %s) %s ($%d, $%d)",
		createdAtColumn,
		idColumn,
		operator,
		placeholderStart,
		placeholderStart+1,
	)

	return where, []any{afterCreatedAt, afterID}, nil
}

func CursorWhere(
	sortColumn string,
	idColumn string,
	sortValue any,
	id uuid.UUID,
	order pagination.Order,
	placeholderStart int,
) (string, []any, error) {
	if !isSafeColumn(sortColumn) {
		return "", nil, fmt.Errorf("%w: %s", ErrUnsafeSQLColumn, sortColumn)
	}
	if !isSafeColumn(idColumn) {
		return "", nil, fmt.Errorf("%w: %s", ErrUnsafeSQLColumn, idColumn)
	}
	if id == uuid.Nil {
		return "", nil, fmt.Errorf("%w: missing cursor id", pagination.ErrInvalidCursor)
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
