package postgres_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/pagination"
)

func TestSortMap_Column(t *testing.T) {
	sortMap := postgres.SortMap{
		"created_at": "contracts.created_at",
		"due_date":   "due_date",
	}

	column, err := sortMap.Column("created_at")
	require.NoError(t, err)
	assert.Equal(t, "contracts.created_at", column)

	_, err = sortMap.Column("unknown")
	require.Error(t, err)
	assert.True(t, errors.Is(err, postgres.ErrInvalidSortField))
}

func TestSortMap_ColumnRejectsUnsafeColumns(t *testing.T) {
	sortMap := postgres.SortMap{
		"evil": "created_at; DROP TABLE contracts",
	}

	_, err := sortMap.Column("evil")
	require.Error(t, err)
	assert.True(t, errors.Is(err, postgres.ErrUnsafeSQLColumn))
}

func TestOrderBy(t *testing.T) {
	sortMap := postgres.SortMap{
		"created_at": "contracts.created_at",
	}

	got, err := postgres.OrderBy(sortMap, "created_at", pagination.OrderDesc, "contracts.id")
	require.NoError(t, err)

	assert.Equal(t, "ORDER BY contracts.created_at DESC, contracts.id DESC", got)
}

func TestOrderByDefaultsInvalidOrder(t *testing.T) {
	sortMap := postgres.SortMap{
		"created_at": "created_at",
	}

	got, err := postgres.OrderBy(sortMap, "created_at", "DROP", "id")
	require.NoError(t, err)

	assert.Equal(t, "ORDER BY created_at DESC, id DESC", got)
}

func TestOrderByRejectsUnsafeTieBreaker(t *testing.T) {
	sortMap := postgres.SortMap{
		"created_at": "created_at",
	}

	_, err := postgres.OrderBy(sortMap, "created_at", pagination.OrderDesc, "id;DROP")
	require.Error(t, err)
	assert.True(t, errors.Is(err, postgres.ErrUnsafeSQLColumn))
}

func TestLimitOffset(t *testing.T) {
	req := pagination.OffsetRequest{
		Page:  3,
		Limit: 20,
	}

	sql, args := postgres.LimitOffset(req, 2)

	assert.Equal(t, "LIMIT $2 OFFSET $3", sql)
	assert.Equal(t, []any{20, 40}, args)
}

func TestLimitOffsetNormalizesRequest(t *testing.T) {
	req := pagination.OffsetRequest{
		Page:  -1,
		Limit: 999,
	}

	sql, args := postgres.LimitOffset(req, 1)

	assert.Equal(t, "LIMIT $1 OFFSET $2", sql)
	assert.Equal(t, []any{pagination.MaxLimit, 0}, args)
}

func TestLimit(t *testing.T) {
	sql, args := postgres.Limit(50, 4)

	assert.Equal(t, "LIMIT $4", sql)
	assert.Equal(t, []any{50}, args)
}

func TestLimitNormalizesBounds(t *testing.T) {
	sql, args := postgres.Limit(0, 1)
	assert.Equal(t, "LIMIT $1", sql)
	assert.Equal(t, []any{pagination.DefaultLimit}, args)

	sql, args = postgres.Limit(999, 1)
	assert.Equal(t, "LIMIT $1", sql)
	assert.Equal(t, []any{pagination.MaxLimit}, args)
}

func TestKeysetWhere_FirstPage(t *testing.T) {
	req := pagination.KeysetRequest{}

	where, args, err := postgres.KeysetWhere("created_at", "id", req, 1)

	require.NoError(t, err)
	assert.Empty(t, where)
	assert.Nil(t, args)
}

func TestKeysetWhere_Desc(t *testing.T) {
	req := pagination.KeysetRequest{
		AfterID:        "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		AfterCreatedAt: "2024-06-01T10:00:00.123456789Z",
		Limit:          20,
		Order:          pagination.OrderDesc,
	}

	where, args, err := postgres.KeysetWhere("contracts.created_at", "contracts.id", req, 5)

	require.NoError(t, err)
	assert.Equal(t, "(contracts.created_at, contracts.id) < ($5, $6)", where)
	require.Len(t, args, 2)

	assert.Equal(t, uuid.MustParse(req.AfterID), args[1])

	gotTime, ok := args[0].(time.Time)
	require.True(t, ok)
	assert.Equal(t, "2024-06-01T10:00:00.123456789Z", gotTime.UTC().Format(time.RFC3339Nano))
}

func TestKeysetWhere_Asc(t *testing.T) {
	req := pagination.KeysetRequest{
		AfterID:        "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		AfterCreatedAt: "2024-06-01T10:00:00Z",
		Order:          pagination.OrderAsc,
	}

	where, args, err := postgres.KeysetWhere("created_at", "id", req, 1)

	require.NoError(t, err)
	assert.Equal(t, "(created_at, id) > ($1, $2)", where)
	require.Len(t, args, 2)
}

func TestKeysetWhereRejectsUnsafeColumns(t *testing.T) {
	req := pagination.KeysetRequest{}

	_, _, err := postgres.KeysetWhere("created_at;DROP", "id", req, 1)
	require.Error(t, err)
	assert.True(t, errors.Is(err, postgres.ErrUnsafeSQLColumn))

	_, _, err = postgres.KeysetWhere("created_at", "id;DROP", req, 1)
	require.Error(t, err)
	assert.True(t, errors.Is(err, postgres.ErrUnsafeSQLColumn))
}

func TestKeysetWhereRejectsInvalidRequest(t *testing.T) {
	req := pagination.KeysetRequest{
		AfterID:        "not-a-uuid",
		AfterCreatedAt: "2024-06-01T10:00:00Z",
	}

	_, _, err := postgres.KeysetWhere("created_at", "id", req, 1)

	require.Error(t, err)
	assert.True(t, errors.Is(err, pagination.ErrInvalidKeyset))
}

func TestCursorWhere_Desc(t *testing.T) {
	id := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")

	where, args, err := postgres.CursorWhere(
		"created_at",
		"id",
		"2024-06-01T10:00:00Z",
		id,
		pagination.OrderDesc,
		3,
	)

	require.NoError(t, err)
	assert.Equal(t, "(created_at, id) < ($3, $4)", where)
	assert.Equal(t, []any{"2024-06-01T10:00:00Z", id}, args)
}

func TestCursorWhere_Asc(t *testing.T) {
	id := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")

	where, args, err := postgres.CursorWhere(
		"created_at",
		"id",
		"2024-06-01T10:00:00Z",
		id,
		pagination.OrderAsc,
		1,
	)

	require.NoError(t, err)
	assert.Equal(t, "(created_at, id) > ($1, $2)", where)
	assert.Equal(t, []any{"2024-06-01T10:00:00Z", id}, args)
}

func TestCursorWhereDefaultsInvalidOrder(t *testing.T) {
	id := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")

	where, _, err := postgres.CursorWhere(
		"created_at",
		"id",
		"2024-06-01T10:00:00Z",
		id,
		"desc;DROP",
		1,
	)

	require.NoError(t, err)
	assert.Equal(t, "(created_at, id) < ($1, $2)", where)
}

func TestCursorWhereRejectsNilID(t *testing.T) {
	_, _, err := postgres.CursorWhere(
		"created_at",
		"id",
		"2024-06-01T10:00:00Z",
		uuid.Nil,
		pagination.OrderDesc,
		1,
	)

	require.Error(t, err)
	assert.True(t, errors.Is(err, pagination.ErrInvalidCursor))
}

func TestCursorWhereRejectsUnsafeColumns(t *testing.T) {
	id := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")

	_, _, err := postgres.CursorWhere("created_at;DROP", "id", "x", id, pagination.OrderDesc, 1)
	require.Error(t, err)
	assert.True(t, errors.Is(err, postgres.ErrUnsafeSQLColumn))

	_, _, err = postgres.CursorWhere("created_at", "id;DROP", "x", id, pagination.OrderDesc, 1)
	require.Error(t, err)
	assert.True(t, errors.Is(err, postgres.ErrUnsafeSQLColumn))
}

func TestSecurity_SQLFragmentsNeverContainUserValues(t *testing.T) {
	id := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	maliciousValue := "2024-06-01T10:00:00Z'; DROP TABLE users; --"

	where, args, err := postgres.CursorWhere(
		"created_at",
		"id",
		maliciousValue,
		id,
		pagination.OrderDesc,
		1,
	)

	require.NoError(t, err)
	assert.NotContains(t, where, maliciousValue)
	assert.Contains(t, where, "$1")
	assert.Contains(t, where, "$2")
	assert.Equal(t, maliciousValue, args[0])
}

func TestSecurity_RejectsCommonUnsafeColumnPatterns(t *testing.T) {
	sortMap := postgres.SortMap{
		"semicolon": "created_at;",
		"comment":   "created_at--",
		"space":     "created_at desc",
		"function":  "lower(name)",
		"quoted":    `"created_at"`,
	}

	for name := range sortMap {
		t.Run(string(name), func(t *testing.T) {
			_, err := sortMap.Column(name)
			require.Error(t, err)
			assert.True(t, errors.Is(err, postgres.ErrUnsafeSQLColumn))
		})
	}
}
