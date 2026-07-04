package pagination_test

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/pagination"
)

func TestOrder_IsValid(t *testing.T) {
	assert.True(t, pagination.OrderAsc.IsValid())
	assert.True(t, pagination.OrderDesc.IsValid())

	assert.False(t, pagination.Order("").IsValid())
	assert.False(t, pagination.Order("ASC").IsValid())
	assert.False(t, pagination.Order("desc;DROP TABLE users").IsValid())
}

func TestOrder_SQL(t *testing.T) {
	assert.Equal(t, "ASC", pagination.OrderAsc.SQL())
	assert.Equal(t, "DESC", pagination.OrderDesc.SQL())

	// Invalid order values must never be reflected into a storage query.
	// SQL() intentionally falls back to DESC for every non-asc value.
	assert.Equal(t, "DESC", pagination.Order("desc;DROP TABLE users").SQL())
	assert.Equal(t, "DESC", pagination.Order("").SQL())
}

func TestSortField_String(t *testing.T) {
	assert.Equal(t, "created_at", pagination.SortField("created_at").String())
	assert.Equal(t, "due_date", pagination.SortField("due_date").String())
}

func TestOffsetRequest_Normalize(t *testing.T) {
	tests := []struct {
		name  string
		input pagination.OffsetRequest
		want  pagination.OffsetRequest
	}{
		{
			name:  "zero value uses defaults",
			input: pagination.OffsetRequest{},
			want: pagination.OffsetRequest{
				Page:  pagination.DefaultPage,
				Limit: pagination.DefaultLimit,
				Sort:  pagination.DefaultSort,
				Order: pagination.DefaultOrder,
			},
		},
		{
			name: "negative page defaults to first page",
			input: pagination.OffsetRequest{
				Page:  -10,
				Limit: 10,
				Sort:  "due_date",
				Order: pagination.OrderDesc,
			},
			want: pagination.OffsetRequest{
				Page:  pagination.DefaultPage,
				Limit: 10,
				Sort:  "due_date",
				Order: pagination.OrderDesc,
			},
		},
		{
			name: "zero limit defaults",
			input: pagination.OffsetRequest{
				Page:  2,
				Limit: 0,
				Sort:  "created_at",
				Order: pagination.OrderAsc,
			},
			want: pagination.OffsetRequest{
				Page:  2,
				Limit: pagination.DefaultLimit,
				Sort:  "created_at",
				Order: pagination.OrderAsc,
			},
		},
		{
			name: "negative limit defaults",
			input: pagination.OffsetRequest{
				Page:  2,
				Limit: -1,
				Sort:  "created_at",
				Order: pagination.OrderAsc,
			},
			want: pagination.OffsetRequest{
				Page:  2,
				Limit: pagination.DefaultLimit,
				Sort:  "created_at",
				Order: pagination.OrderAsc,
			},
		},
		{
			name: "limit over max is clamped",
			input: pagination.OffsetRequest{
				Page:  1,
				Limit: 999,
				Sort:  "created_at",
				Order: pagination.OrderAsc,
			},
			want: pagination.OffsetRequest{
				Page:  1,
				Limit: pagination.MaxLimit,
				Sort:  "created_at",
				Order: pagination.OrderAsc,
			},
		},
		{
			name: "empty sort defaults",
			input: pagination.OffsetRequest{
				Page:  1,
				Limit: 10,
				Sort:  "",
				Order: pagination.OrderAsc,
			},
			want: pagination.OffsetRequest{
				Page:  1,
				Limit: 10,
				Sort:  pagination.DefaultSort,
				Order: pagination.OrderAsc,
			},
		},
		{
			name: "invalid order defaults",
			input: pagination.OffsetRequest{
				Page:  1,
				Limit: 10,
				Sort:  "created_at",
				Order: "random",
			},
			want: pagination.OffsetRequest{
				Page:  1,
				Limit: 10,
				Sort:  "created_at",
				Order: pagination.DefaultOrder,
			},
		},
		{
			name: "explicit valid values unchanged",
			input: pagination.OffsetRequest{
				Page:  3,
				Limit: 50,
				Sort:  "due_date",
				Order: pagination.OrderAsc,
			},
			want: pagination.OffsetRequest{
				Page:  3,
				Limit: 50,
				Sort:  "due_date",
				Order: pagination.OrderAsc,
			},
		},
		{
			name: "dangerous sort value is preserved for repository whitelist rejection",
			input: pagination.OffsetRequest{
				Page:  1,
				Limit: 10,
				Sort:  "created_at;DROP TABLE users",
				Order: pagination.OrderDesc,
			},
			want: pagination.OffsetRequest{
				Page:  1,
				Limit: 10,
				Sort:  "created_at;DROP TABLE users",
				Order: pagination.OrderDesc,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.input
			got.Normalize()

			assert.Equal(t, tc.want, got)
		})
	}
}

func TestOffsetRequest_SQLOffset(t *testing.T) {
	tests := []struct {
		name string
		req  pagination.OffsetRequest
		want int
	}{
		{
			name: "first page",
			req:  pagination.OffsetRequest{Page: 1, Limit: 20},
			want: 0,
		},
		{
			name: "second page",
			req:  pagination.OffsetRequest{Page: 2, Limit: 20},
			want: 20,
		},
		{
			name: "third page",
			req:  pagination.OffsetRequest{Page: 3, Limit: 20},
			want: 40,
		},
		{
			name: "large page",
			req:  pagination.OffsetRequest{Page: 100, Limit: 100},
			want: 9900,
		},
		{
			name: "page zero returns zero",
			req:  pagination.OffsetRequest{Page: 0, Limit: 20},
			want: 0,
		},
		{
			name: "negative page returns zero",
			req:  pagination.OffsetRequest{Page: -1, Limit: 20},
			want: 0,
		},
		{
			name: "zero limit returns zero",
			req:  pagination.OffsetRequest{Page: 3, Limit: 0},
			want: 0,
		},
		{
			name: "negative limit returns zero",
			req:  pagination.OffsetRequest{Page: 3, Limit: -1},
			want: 0,
		},
		{
			name: "overflow saturates to max int",
			req:  pagination.OffsetRequest{Page: math.MaxInt, Limit: 2},
			want: math.MaxInt,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.req.SQLOffset())
		})
	}
}

func TestNewPageMeta(t *testing.T) {
	tests := []struct {
		name        string
		req         pagination.OffsetRequest
		total       int64
		totalPages  int
		hasNext     bool
		hasPrevious bool
	}{
		{
			name:        "empty result",
			req:         pagination.OffsetRequest{Page: 1, Limit: 20},
			total:       0,
			totalPages:  0,
			hasNext:     false,
			hasPrevious: false,
		},
		{
			name:        "first page has next",
			req:         pagination.OffsetRequest{Page: 1, Limit: 20},
			total:       45,
			totalPages:  3,
			hasNext:     true,
			hasPrevious: false,
		},
		{
			name:        "middle page has both directions",
			req:         pagination.OffsetRequest{Page: 2, Limit: 20},
			total:       45,
			totalPages:  3,
			hasNext:     true,
			hasPrevious: true,
		},
		{
			name:        "last page has previous only",
			req:         pagination.OffsetRequest{Page: 3, Limit: 20},
			total:       45,
			totalPages:  3,
			hasNext:     false,
			hasPrevious: true,
		},
		{
			name:        "exact division",
			req:         pagination.OffsetRequest{Page: 2, Limit: 20},
			total:       40,
			totalPages:  2,
			hasNext:     false,
			hasPrevious: true,
		},
		{
			name:        "zero limit does not panic",
			req:         pagination.OffsetRequest{Page: 1, Limit: 0},
			total:       40,
			totalPages:  0,
			hasNext:     false,
			hasPrevious: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := pagination.NewPageMeta(tc.req, tc.total)

			assert.Equal(t, "page", got.PaginationType)
			assert.Equal(t, tc.req.Page, got.Page)
			assert.Equal(t, tc.req.Limit, got.Limit)
			assert.Equal(t, tc.total, got.Total)
			assert.Equal(t, tc.totalPages, got.TotalPages)
			assert.Equal(t, tc.hasNext, got.HasNext)
			assert.Equal(t, tc.hasPrevious, got.HasPrevious)
		})
	}
}

func TestCursorRequest_Normalize(t *testing.T) {
	tests := []struct {
		name  string
		input pagination.CursorRequest
		want  pagination.CursorRequest
	}{
		{
			name:  "zero value uses defaults",
			input: pagination.CursorRequest{},
			want: pagination.CursorRequest{
				Limit: pagination.DefaultLimit,
				Sort:  pagination.DefaultSort,
				Order: pagination.DefaultOrder,
			},
		},
		{
			name: "limit over max is clamped",
			input: pagination.CursorRequest{
				Cursor: "cursor",
				Limit:  999,
				Sort:   "due_date",
				Order:  pagination.OrderAsc,
			},
			want: pagination.CursorRequest{
				Cursor: "cursor",
				Limit:  pagination.MaxLimit,
				Sort:   "due_date",
				Order:  pagination.OrderAsc,
			},
		},
		{
			name: "invalid order defaults",
			input: pagination.CursorRequest{
				Limit: 10,
				Sort:  "created_at",
				Order: "desc;DROP TABLE users",
			},
			want: pagination.CursorRequest{
				Limit: 10,
				Sort:  "created_at",
				Order: pagination.DefaultOrder,
			},
		},
		{
			name: "explicit valid values unchanged",
			input: pagination.CursorRequest{
				Cursor: "cursor",
				Limit:  50,
				Sort:   "due_date",
				Order:  pagination.OrderAsc,
			},
			want: pagination.CursorRequest{
				Cursor: "cursor",
				Limit:  50,
				Sort:   "due_date",
				Order:  pagination.OrderAsc,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.input
			got.Normalize()

			assert.Equal(t, tc.want, got)
		})
	}
}

func TestKeysetRequest_Normalize(t *testing.T) {
	tests := []struct {
		name  string
		input pagination.KeysetRequest
		want  pagination.KeysetRequest
	}{
		{
			name:  "zero value uses defaults",
			input: pagination.KeysetRequest{},
			want: pagination.KeysetRequest{
				Limit: pagination.DefaultLimit,
				Order: pagination.DefaultOrder,
			},
		},
		{
			name: "limit over max is clamped",
			input: pagination.KeysetRequest{
				AfterID:        "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
				AfterCreatedAt: "2024-06-01T10:00:00Z",
				Limit:          999,
				Order:          pagination.OrderAsc,
			},
			want: pagination.KeysetRequest{
				AfterID:        "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
				AfterCreatedAt: "2024-06-01T10:00:00Z",
				Limit:          pagination.MaxLimit,
				Order:          pagination.OrderAsc,
			},
		},
		{
			name: "invalid order defaults",
			input: pagination.KeysetRequest{
				Limit: 10,
				Order: "random",
			},
			want: pagination.KeysetRequest{
				Limit: 10,
				Order: pagination.DefaultOrder,
			},
		},
		{
			name: "explicit valid values unchanged",
			input: pagination.KeysetRequest{
				AfterID:        "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
				AfterCreatedAt: "2024-06-01T10:00:00Z",
				Limit:          10,
				Order:          pagination.OrderAsc,
			},
			want: pagination.KeysetRequest{
				AfterID:        "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
				AfterCreatedAt: "2024-06-01T10:00:00Z",
				Limit:          10,
				Order:          pagination.OrderAsc,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.input
			got.Normalize()

			assert.Equal(t, tc.want, got)
		})
	}
}

func TestKeysetRequest_HasAfter(t *testing.T) {
	assert.False(t, (pagination.KeysetRequest{}).HasAfter())
	assert.False(t, (pagination.KeysetRequest{AfterID: "id"}).HasAfter())
	assert.False(t, (pagination.KeysetRequest{AfterCreatedAt: "ts"}).HasAfter())
	assert.False(t, (pagination.KeysetRequest{AfterDueAt: "ts"}).HasAfter())
	assert.True(t, (pagination.KeysetRequest{AfterID: "id", AfterCreatedAt: "ts"}).HasAfter())
	assert.True(t, (pagination.KeysetRequest{AfterID: "id", AfterDueAt: "ts"}).HasAfter())
}

func TestKeysetRequest_Validate(t *testing.T) {
	validID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	validTS := "2024-06-01T10:00:00.123456789Z"

	tests := []struct {
		name    string
		req     pagination.KeysetRequest
		wantErr bool
	}{
		{
			name:    "empty after is valid first page",
			req:     pagination.KeysetRequest{},
			wantErr: false,
		},
		{
			name: "valid after created_at pair",
			req: pagination.KeysetRequest{
				AfterID:        validID,
				AfterCreatedAt: validTS,
			},
			wantErr: false,
		},
		{
			name: "valid after due_at pair",
			req: pagination.KeysetRequest{
				AfterID:    validID,
				AfterDueAt: validTS,
			},
			wantErr: false,
		},
		{
			name: "missing timestamp",
			req: pagination.KeysetRequest{
				AfterID: validID,
			},
			wantErr: true,
		},
		{
			name: "missing id",
			req: pagination.KeysetRequest{
				AfterCreatedAt: validTS,
			},
			wantErr: true,
		},
		{
			name: "both timestamps rejected",
			req: pagination.KeysetRequest{
				AfterID:        validID,
				AfterCreatedAt: validTS,
				AfterDueAt:     validTS,
			},
			wantErr: true,
		},
		{
			name: "invalid uuid",
			req: pagination.KeysetRequest{
				AfterID:        "not-a-uuid",
				AfterCreatedAt: validTS,
			},
			wantErr: true,
		},
		{
			name: "malicious uuid is rejected",
			req: pagination.KeysetRequest{
				AfterID:        "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee'; DROP TABLE users; --",
				AfterCreatedAt: validTS,
			},
			wantErr: true,
		},
		{
			name: "invalid timestamp",
			req: pagination.KeysetRequest{
				AfterID:        validID,
				AfterCreatedAt: "not-a-time",
			},
			wantErr: true,
		},
		{
			name: "malicious timestamp is rejected",
			req: pagination.KeysetRequest{
				AfterID:        validID,
				AfterCreatedAt: "2024-06-01T10:00:00Z'; DROP TABLE users; --",
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.req.Validate()

			if tc.wantErr {
				require.Error(t, err)
				assert.True(t, errors.Is(err, pagination.ErrInvalidKeyset))
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestNewCursor(t *testing.T) {
	id := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")

	got := pagination.NewCursor("", id, "", "invalid")

	assert.Equal(t, "", got.SortVal)
	assert.Equal(t, id, got.ID)
	assert.Equal(t, pagination.DefaultSort, got.Sort)
	assert.Equal(t, pagination.DefaultOrder, got.Order)
}

func TestCursor_EncodeDecode_RoundTrip(t *testing.T) {
	id := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	cursor := pagination.NewCursor(
		"2024-06-01T10:00:00.123456789Z",
		id,
		pagination.SortField("created_at"),
		pagination.OrderDesc,
	)

	token, err := cursor.Encode()
	require.NoError(t, err)
	require.NotEmpty(t, token)

	// Raw URL encoding must not include padding or URL-unsafe characters.
	assert.False(t, strings.ContainsAny(token, "+/="), "cursor token must be URL-safe")

	got, err := pagination.DecodeCursor(token)
	require.NoError(t, err)

	assert.Equal(t, cursor, got)
}

func TestCursor_Encode_ValidationErrors(t *testing.T) {
	id := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")

	tests := []struct {
		name   string
		cursor pagination.Cursor
	}{
		{
			name: "missing sort value",
			cursor: pagination.Cursor{
				ID:    id,
				Sort:  pagination.DefaultSort,
				Order: pagination.DefaultOrder,
			},
		},
		{
			name: "missing id",
			cursor: pagination.Cursor{
				SortVal: "2024-06-01T10:00:00Z",
				Sort:    pagination.DefaultSort,
				Order:   pagination.DefaultOrder,
			},
		},
		{
			name: "missing sort field",
			cursor: pagination.Cursor{
				SortVal: "2024-06-01T10:00:00Z",
				ID:      id,
				Order:   pagination.DefaultOrder,
			},
		},
		{
			name: "invalid order",
			cursor: pagination.Cursor{
				SortVal: "2024-06-01T10:00:00Z",
				ID:      id,
				Sort:    pagination.DefaultSort,
				Order:   "desc;DROP TABLE users",
			},
		},
		{
			name: "unsupported version",
			cursor: pagination.Cursor{
				Version: 999,
				SortVal: "2024-06-01T10:00:00Z",
				ID:      id,
				Sort:    pagination.DefaultSort,
				Order:   pagination.DefaultOrder,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			token, err := tc.cursor.Encode()

			require.Error(t, err)
			assert.Empty(t, token)
			assert.True(t, errors.Is(err, pagination.ErrInvalidCursor))
		})
	}
}

func TestCursor_Validate(t *testing.T) {
	id := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")

	cursor := pagination.NewCursor("2024-06-01T10:00:00Z", id, pagination.DefaultSort, pagination.DefaultOrder)
	require.NoError(t, cursor.Validate())
}

func TestDecodeCursor_EmptyString(t *testing.T) {
	got, err := pagination.DecodeCursor("")

	require.NoError(t, err)
	assert.Equal(t, pagination.Cursor{}, got)
}

func TestDecodeCursor_WhitespaceOnly(t *testing.T) {
	got, err := pagination.DecodeCursor("   \n\t  ")

	require.NoError(t, err)
	assert.Equal(t, pagination.Cursor{}, got)
}

func TestDecodeCursor_InvalidBase64(t *testing.T) {
	_, err := pagination.DecodeCursor("not-valid-base64!!!")

	require.Error(t, err)
	assert.True(t, errors.Is(err, pagination.ErrInvalidCursor))
}

func TestDecodeCursor_InvalidJSON(t *testing.T) {
	token := base64.RawURLEncoding.EncodeToString([]byte("{not-json"))

	_, err := pagination.DecodeCursor(token)

	require.Error(t, err)
	assert.True(t, errors.Is(err, pagination.ErrInvalidCursor))
}

func TestDecodeCursor_ValidJSONButMissingRequiredFields(t *testing.T) {
	token := base64.RawURLEncoding.EncodeToString([]byte(`{}`))

	_, err := pagination.DecodeCursor(token)

	require.Error(t, err)
	assert.True(t, errors.Is(err, pagination.ErrInvalidCursor))
}

func TestDecodeCursor_UnsupportedVersion(t *testing.T) {
	id := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	token := mustCursorToken(t, map[string]any{
		"ver": 999,
		"v":   "2024-06-01T10:00:00Z",
		"id":  id.String(),
		"s":   "created_at",
		"o":   "desc",
	})

	_, err := pagination.DecodeCursor(token)

	require.Error(t, err)
	assert.True(t, errors.Is(err, pagination.ErrInvalidCursor))
	assert.Contains(t, err.Error(), "unsupported cursor version")
}

func TestDecodeCursor_MissingSortValue(t *testing.T) {
	id := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	token := mustCursorToken(t, map[string]any{
		"ver": 1,
		"id":  id.String(),
		"s":   "created_at",
		"o":   "desc",
	})

	_, err := pagination.DecodeCursor(token)

	require.Error(t, err)
	assert.True(t, errors.Is(err, pagination.ErrInvalidCursor))
}

func TestDecodeCursor_MissingID(t *testing.T) {
	token := mustCursorToken(t, map[string]any{
		"ver": 1,
		"v":   "2024-06-01T10:00:00Z",
		"s":   "created_at",
		"o":   "desc",
	})

	_, err := pagination.DecodeCursor(token)

	require.Error(t, err)
	assert.True(t, errors.Is(err, pagination.ErrInvalidCursor))
}

func TestDecodeCursor_InvalidUUID(t *testing.T) {
	token := mustCursorToken(t, map[string]any{
		"ver": 1,
		"v":   "2024-06-01T10:00:00Z",
		"id":  "not-a-uuid",
		"s":   "created_at",
		"o":   "desc",
	})

	_, err := pagination.DecodeCursor(token)

	require.Error(t, err)
	assert.True(t, errors.Is(err, pagination.ErrInvalidCursor))
}

func TestDecodeCursor_MissingSortField(t *testing.T) {
	id := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	token := mustCursorToken(t, map[string]any{
		"ver": 1,
		"v":   "2024-06-01T10:00:00Z",
		"id":  id.String(),
		"o":   "desc",
	})

	_, err := pagination.DecodeCursor(token)

	require.Error(t, err)
	assert.True(t, errors.Is(err, pagination.ErrInvalidCursor))
}

func TestDecodeCursor_InvalidOrder(t *testing.T) {
	id := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	token := mustCursorToken(t, map[string]any{
		"ver": 1,
		"v":   "2024-06-01T10:00:00Z",
		"id":  id.String(),
		"s":   "created_at",
		"o":   "desc;DROP TABLE users",
	})

	_, err := pagination.DecodeCursor(token)

	require.Error(t, err)
	assert.True(t, errors.Is(err, pagination.ErrInvalidCursor))
}

func TestDecodeCursor_ExtraJSONFieldsAreIgnored(t *testing.T) {
	id := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	token := mustCursorToken(t, map[string]any{
		"ver":        1,
		"v":          "2024-06-01T10:00:00Z",
		"id":         id.String(),
		"s":          "created_at",
		"o":          "desc",
		"unexpected": "ignored",
	})

	got, err := pagination.DecodeCursor(token)

	require.NoError(t, err)
	assert.Equal(t, "2024-06-01T10:00:00Z", got.SortVal)
	assert.Equal(t, id, got.ID)
	assert.Equal(t, pagination.SortField("created_at"), got.Sort)
	assert.Equal(t, pagination.OrderDesc, got.Order)
}

func TestDecodeCursor_MaliciousSortFieldStillDecodes(t *testing.T) {
	id := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	token := mustCursorToken(t, map[string]any{
		"ver": 1,
		"v":   "2024-06-01T10:00:00Z",
		"id":  id.String(),
		"s":   "created_at;DROP TABLE users",
		"o":   "desc",
	})

	got, err := pagination.DecodeCursor(token)

	require.NoError(t, err)
	assert.Equal(t, pagination.SortField("created_at;DROP TABLE users"), got.Sort)
}

func TestTypedPages(t *testing.T) {
	offsetPage := pagination.OffsetPage[string]{
		Data: []string{"a", "b"},
		Meta: pagination.PageMeta{PaginationType: "page", Limit: 2},
	}
	assert.Equal(t, []string{"a", "b"}, offsetPage.Data)
	assert.Equal(t, "page", offsetPage.Meta.PaginationType)

	nextCursor := "cursor-token"
	cursorPage := pagination.CursorPage[int]{
		Data: []int{1, 2},
		Meta: pagination.CursorMeta{
			PaginationType: "cursor",
			Limit:          2,
			NextCursor:     &nextCursor,
			HasNext:        true,
		},
	}
	require.NotNil(t, cursorPage.Meta.NextCursor)
	assert.Equal(t, nextCursor, *cursorPage.Meta.NextCursor)

	nextID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	nextCreatedAt := "2024-06-01T10:00:00Z"
	keysetPage := pagination.KeysetPage[int]{
		Data: []int{1, 2},
		Meta: pagination.KeysetMeta{
			PaginationType:     "keyset",
			Limit:              2,
			NextAfterID:        &nextID,
			NextAfterCreatedAt: &nextCreatedAt,
			HasNext:            true,
		},
	}
	require.NotNil(t, keysetPage.Meta.NextAfterID)
	require.NotNil(t, keysetPage.Meta.NextAfterCreatedAt)
	assert.Equal(t, nextID, *keysetPage.Meta.NextAfterID)
	assert.Equal(t, nextCreatedAt, *keysetPage.Meta.NextAfterCreatedAt)
}

func mustCursorToken(t *testing.T, payload map[string]any) string {
	t.Helper()

	b, err := json.Marshal(payload)
	require.NoError(t, err)

	return base64.RawURLEncoding.EncodeToString(b)
}
