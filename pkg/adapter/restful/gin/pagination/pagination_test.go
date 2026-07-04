package pagination_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	adapterpagination "github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/pagination"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	corepagination "github.com/parstis/bnpl-backend/pkg/core/pagination"
	"github.com/parstis/bnpl-backend/pkg/core/trace"
)

func TestParseOffset_Defaults(t *testing.T) {
	c, w := newTestContext("/?")

	got, ok := adapterpagination.ParseOffset(c)

	require.True(t, ok)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, corepagination.OffsetRequest{
		Page:  corepagination.DefaultPage,
		Limit: corepagination.DefaultLimit,
		Sort:  corepagination.DefaultSort,
		Order: corepagination.DefaultOrder,
	}, got)
}

func TestParseOffset_ValidValues(t *testing.T) {
	c, w := newTestContext("/?page=3&limit=50&sort=due_date&order=asc")

	got, ok := adapterpagination.ParseOffset(c)

	require.True(t, ok)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, corepagination.OffsetRequest{
		Page:  3,
		Limit: 50,
		Sort:  corepagination.SortField("due_date"),
		Order: corepagination.OrderAsc,
	}, got)
}

func TestParseOffset_TrimsWhitespace(t *testing.T) {
	c, _ := newTestContext("/?page=%203%20&limit=%205%20&sort=%20created_at%20&order=%20desc%20")

	got, ok := adapterpagination.ParseOffset(c)

	require.True(t, ok)
	assert.Equal(t, 3, got.Page)
	assert.Equal(t, 5, got.Limit)
	assert.Equal(t, corepagination.SortField("created_at"), got.Sort)
	assert.Equal(t, corepagination.OrderDesc, got.Order)
}

func TestParseOffset_InvalidPage(t *testing.T) {
	tests := []string{
		"/?page=0",
		"/?page=-1",
		"/?page=abc",
		"/?page=1.5",
		"/?page=1%3BDROP%20TABLE%20users",
	}

	for _, target := range tests {
		t.Run(target, func(t *testing.T) {
			c, w := newTestContext(target)

			got, ok := adapterpagination.ParseOffset(c)

			require.False(t, ok)
			assert.Equal(t, corepagination.OffsetRequest{}, got)
			assertValidationResponse(t, w, "page")
		})
	}
}

func TestParseOffset_InvalidLimit(t *testing.T) {
	tests := []string{
		"/?limit=0",
		"/?limit=-1",
		"/?limit=abc",
		"/?limit=1.5",
		"/?limit=1%3BDROP%20TABLE%20users",
	}

	for _, target := range tests {
		t.Run(target, func(t *testing.T) {
			c, w := newTestContext(target)

			got, ok := adapterpagination.ParseOffset(c)

			require.False(t, ok)
			assert.Equal(t, corepagination.OffsetRequest{}, got)
			assertValidationResponse(t, w, "limit")
		})
	}
}

func TestParseOffset_LimitOverMaxIsClamped(t *testing.T) {
	c, _ := newTestContext("/?limit=999")

	got, ok := adapterpagination.ParseOffset(c)

	require.True(t, ok)
	assert.Equal(t, corepagination.MaxLimit, got.Limit)
}

func TestParseOffset_InvalidOrder(t *testing.T) {
	tests := []string{
		"/?order=random",
		"/?order=ASC",
		"/?order=desc%3BDROP%20TABLE%20users",
	}

	for _, target := range tests {
		t.Run(target, func(t *testing.T) {
			c, w := newTestContext(target)

			got, ok := adapterpagination.ParseOffset(c)

			require.False(t, ok)
			assert.Equal(t, corepagination.OffsetRequest{}, got)
			assertValidationResponse(t, w, "order")
		})
	}
}

func TestParseOffset_DangerousSortIsPreservedForRepositoryWhitelist(t *testing.T) {
	c, _ := newTestContext("/?sort=created_at%3BDROP%20TABLE%20users&order=desc")

	got, ok := adapterpagination.ParseOffset(c)

	require.True(t, ok)
	assert.Equal(t, corepagination.SortField("created_at;DROP TABLE users"), got.Sort)
	assert.Equal(t, corepagination.OrderDesc, got.Order)
}

func TestParseCursor_Defaults(t *testing.T) {
	c, w := newTestContext("/?")

	got, ok := adapterpagination.ParseCursor(c)

	require.True(t, ok)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, corepagination.CursorRequest{
		Limit: corepagination.DefaultLimit,
		Sort:  corepagination.DefaultSort,
		Order: corepagination.DefaultOrder,
	}, got)
}

func TestParseCursor_ValidValues(t *testing.T) {
	token := validCursorToken(t)
	c, _ := newTestContext("/?cursor=" + token + "&limit=10&sort=created_at&order=desc")

	got, ok := adapterpagination.ParseCursor(c)

	require.True(t, ok)
	assert.Equal(t, token, got.Cursor)
	assert.Equal(t, 10, got.Limit)
	assert.Equal(t, corepagination.DefaultSort, got.Sort)
	assert.Equal(t, corepagination.OrderDesc, got.Order)
}

func TestParseCursor_TrimsWhitespace(t *testing.T) {
	token := validCursorToken(t)
	c, _ := newTestContext("/?cursor=%20" + token + "%20&limit=%2010%20&sort=%20created_at%20&order=%20desc%20")

	got, ok := adapterpagination.ParseCursor(c)

	require.True(t, ok)
	assert.Equal(t, token, got.Cursor)
	assert.Equal(t, 10, got.Limit)
	assert.Equal(t, corepagination.DefaultSort, got.Sort)
	assert.Equal(t, corepagination.OrderDesc, got.Order)
}

func TestParseCursor_InvalidLimit(t *testing.T) {
	tests := []string{
		"/?limit=0",
		"/?limit=-1",
		"/?limit=abc",
		"/?limit=1.5",
		"/?limit=1%3BDROP%20TABLE%20users",
	}

	for _, target := range tests {
		t.Run(target, func(t *testing.T) {
			c, w := newTestContext(target)

			got, ok := adapterpagination.ParseCursor(c)

			require.False(t, ok)
			assert.Equal(t, corepagination.CursorRequest{}, got)
			assertValidationResponse(t, w, "limit")
		})
	}
}

func TestParseCursor_LimitOverMaxIsClamped(t *testing.T) {
	c, _ := newTestContext("/?limit=999")

	got, ok := adapterpagination.ParseCursor(c)

	require.True(t, ok)
	assert.Equal(t, corepagination.MaxLimit, got.Limit)
}

func TestParseCursor_InvalidOrder(t *testing.T) {
	tests := []string{
		"/?order=random",
		"/?order=ASC",
		"/?order=desc%3BDROP%20TABLE%20users",
	}

	for _, target := range tests {
		t.Run(target, func(t *testing.T) {
			c, w := newTestContext(target)

			got, ok := adapterpagination.ParseCursor(c)

			require.False(t, ok)
			assert.Equal(t, corepagination.CursorRequest{}, got)
			assertValidationResponse(t, w, "order")
		})
	}
}

func TestParseCursor_InvalidCursor(t *testing.T) {
	tests := []string{
		"/?cursor=not-valid-base64!!!",
		"/?cursor=e30", // {}
		"/?cursor=eyJ2ZXIiOjk5OSwidmFsdWUiOiJ4In0",
	}

	for _, target := range tests {
		t.Run(target, func(t *testing.T) {
			c, w := newTestContext(target)

			got, ok := adapterpagination.ParseCursor(c)

			require.False(t, ok)
			assert.Equal(t, corepagination.CursorRequest{}, got)
			assertValidationResponse(t, w, "cursor")
		})
	}
}

func TestParseKeyset_Defaults(t *testing.T) {
	c, w := newTestContext("/?")

	got, ok := adapterpagination.ParseKeyset(c)

	require.True(t, ok)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, corepagination.KeysetRequest{
		Limit: corepagination.DefaultLimit,
		Order: corepagination.DefaultOrder,
	}, got)
}

func TestParseKeyset_ValidValues(t *testing.T) {
	target := "/?after_id=aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee&after_created_at=2024-06-01T10:00:00.123456789Z&limit=10&order=asc"
	c, _ := newTestContext(target)

	got, ok := adapterpagination.ParseKeyset(c)

	require.True(t, ok)
	assert.Equal(t, "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", got.AfterID)
	assert.Equal(t, "2024-06-01T10:00:00.123456789Z", got.AfterCreatedAt)
	assert.Equal(t, 10, got.Limit)
	assert.Equal(t, corepagination.OrderAsc, got.Order)
}

func TestParseKeyset_TrimsWhitespace(t *testing.T) {
	target := "/?after_id=%20aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee%20&after_created_at=%202024-06-01T10:00:00Z%20&limit=%2010%20&order=%20desc%20"
	c, _ := newTestContext(target)

	got, ok := adapterpagination.ParseKeyset(c)

	require.True(t, ok)
	assert.Equal(t, "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", got.AfterID)
	assert.Equal(t, "2024-06-01T10:00:00Z", got.AfterCreatedAt)
	assert.Equal(t, 10, got.Limit)
	assert.Equal(t, corepagination.OrderDesc, got.Order)
}

func TestParseKeyset_InvalidLimit(t *testing.T) {
	tests := []string{
		"/?limit=0",
		"/?limit=-1",
		"/?limit=abc",
		"/?limit=1.5",
		"/?limit=1%3BDROP%20TABLE%20users",
	}

	for _, target := range tests {
		t.Run(target, func(t *testing.T) {
			c, w := newTestContext(target)

			got, ok := adapterpagination.ParseKeyset(c)

			require.False(t, ok)
			assert.Equal(t, corepagination.KeysetRequest{}, got)
			assertValidationResponse(t, w, "limit")
		})
	}
}

func TestParseKeyset_LimitOverMaxIsClamped(t *testing.T) {
	c, _ := newTestContext("/?limit=999")

	got, ok := adapterpagination.ParseKeyset(c)

	require.True(t, ok)
	assert.Equal(t, corepagination.MaxLimit, got.Limit)
}

func TestParseKeyset_InvalidOrder(t *testing.T) {
	tests := []string{
		"/?order=random",
		"/?order=ASC",
		"/?order=desc%3BDROP%20TABLE%20users",
	}

	for _, target := range tests {
		t.Run(target, func(t *testing.T) {
			c, w := newTestContext(target)

			got, ok := adapterpagination.ParseKeyset(c)

			require.False(t, ok)
			assert.Equal(t, corepagination.KeysetRequest{}, got)
			assertValidationResponse(t, w, "order")
		})
	}
}

func TestParseKeyset_InvalidAfterPair(t *testing.T) {
	tests := []string{
		"/?after_id=aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		"/?after_created_at=2024-06-01T10:00:00Z",
		"/?after_id=not-a-uuid&after_created_at=2024-06-01T10:00:00Z",
		"/?after_id=aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee&after_created_at=not-a-time",
		"/?after_id=aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee%27%3B%20DROP%20TABLE%20users%3B--&after_created_at=2024-06-01T10:00:00Z",
		"/?after_id=aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee&after_created_at=2024-06-01T10:00:00Z%27%3B%20DROP%20TABLE%20users%3B--",
	}

	for _, target := range tests {
		t.Run(target, func(t *testing.T) {
			c, w := newTestContext(target)

			got, ok := adapterpagination.ParseKeyset(c)

			require.False(t, ok)
			assert.Equal(t, corepagination.KeysetRequest{}, got)
			assertValidationResponse(t, w, "after")
		})
	}
}

func newTestContext(target string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	ctx := context.Background()
	ctx = trace.WithTraceID(ctx, "trace-test")
	ctx = trace.WithRequestID(ctx, "request-test")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		panic(err)
	}

	c.Request = req
	return c, w
}

func validCursorToken(t *testing.T) string {
	t.Helper()

	id := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	cursor := corepagination.NewCursor(
		"2024-06-01T10:00:00Z",
		id,
		corepagination.DefaultSort,
		corepagination.DefaultOrder,
	)

	token, err := cursor.Encode()
	require.NoError(t, err)

	return token
}

func assertValidationResponse(t *testing.T, w *httptest.ResponseRecorder, field string) {
	t.Helper()

	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)

	var resp serdser.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, string(cerr.CodeValidationFailed), resp.Code)
	assert.Equal(t, "trace-test", resp.TraceID)
	require.NotEmpty(t, resp.Details.Fields)
	assert.Contains(t, resp.Details.Fields, field)
}
