package serdser

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/header"
	resti18n "github.com/parstis/bnpl-backend/pkg/adapter/restful/i18n"
	i18nbuiltin "github.com/parstis/bnpl-backend/pkg/adapter/restful/i18n/builtin"
	corepagination "github.com/parstis/bnpl-backend/pkg/core/pagination"
	"github.com/parstis/bnpl-backend/pkg/core/trace"
)

func TestSuccessMessageKeys_AreLocalizedInEnglishAndPersian(t *testing.T) {
	t.Parallel()

	localizer, err := i18nbuiltin.New(resti18n.LocaleENUS)
	require.NoError(t, err)

	keys := []SuccessMessageKey{
		SuccessMessageDefault,
		SuccessMessageActiveContractsListed,
		SuccessMessageCustomerCreditNotAssigned,
		SuccessMessageCustomerCreditExpired,
		SuccessMessageCustomerCreditSummaryReturned,
		SuccessMessageCheckoutDecisionAlreadyExists,
		SuccessMessageCheckoutDecisionCreated,
		SuccessMessageContractInstallmentsListed,
		SuccessMessageInitialCustomerCreditLimitAssigned,
		SuccessMessageInitialCustomerCreditLimitAssignmentDenied,
		SuccessMessageInitialCustomerCreditLimitAlreadyActive,
		SuccessMessageInstallmentPaymentAttemptAlreadyExists,
		SuccessMessageInstallmentPaymentAttemptQueued,
		SuccessMessageInstallmentPaymentHistoryListed,
		SuccessMessageMerchantBranchesListed,
		SuccessMessageMerchantProfileReturned,
		SuccessMessageMerchantSalesListed,
		SuccessMessageContractAlreadyExists,
		SuccessMessageContractCreated,
		SuccessMessageUnpaidInstallmentsListed,
	}

	for _, locale := range []resti18n.Locale{resti18n.LocaleENUS, resti18n.LocaleFAIR} {
		for _, key := range keys {
			message, localizeErr := localizer.Localize(locale, key.String(), nil)
			require.NoError(t, localizeErr, "locale=%s key=%s", locale, key.String())
			assert.NotEmpty(t, message, "locale=%s key=%s", locale, key.String())
		}
	}
}

func TestSer_IncludesTraceMetadata(t *testing.T) {
	t.Parallel()

	c, w := newSuccessContext("trace-1", "request-1", "correlation-1")

	Ser(c, http.StatusOK, SuccessCodeHealthOK, SuccessMessageDefault, map[string]string{"status": "UP"})

	var resp SuccessResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, http.StatusOK, w.Code)
	data, ok := resp.Data.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "UP", data["status"])
	assert.Equal(t, "success", resp.Meta.Status)
	assert.Equal(t, string(SuccessCodeHealthOK), resp.Meta.Code)
	assert.Equal(t, "Request completed successfully.", resp.Meta.Message)
	assert.Equal(t, http.StatusOK, resp.Meta.StatusCode)
	assert.Equal(t, "trace-1", resp.Meta.TraceID)
	assert.Equal(t, "request-1", resp.Meta.RequestID)
	require.NotNil(t, resp.Meta.CorrelationID)
	assert.Equal(t, "correlation-1", *resp.Meta.CorrelationID)
	assert.NotEmpty(t, resp.Meta.Timestamp)
	assert.Nil(t, resp.Meta.Pagination)
}

func TestSer_LocalizesSuccessfulResponseFromLangHeader(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	localizer, err := i18nbuiltin.New(resti18n.LocaleENUS)
	require.NoError(t, err)

	engine := gin.New()
	engine.Use(testLanguageMiddleware(localizer, resti18n.LocaleENUS))
	engine.GET("/customer-credit", func(c *gin.Context) {
		Ser(
			c,
			http.StatusOK,
			SuccessCodeCustomerCreditNotAssigned,
			SuccessMessageCustomerCreditNotAssigned,
			map[string]string{"status": "not_assigned"},
		)
	})

	req := httptest.NewRequest(http.MethodGet, "/customer-credit", nil)
	req.Header.Set(header.Language, "fa-IR")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	var resp SuccessResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "fa-IR", rec.Header().Get(header.ContentLanguage))
	assert.Equal(t, "success", resp.Meta.Status)
	assert.Equal(t, string(SuccessCodeCustomerCreditNotAssigned), resp.Meta.Code)
	assert.Equal(t, "اعتباری برای مشتری تخصیص داده نشده است؛ ارزیابی اعتبار لازم است.", resp.Meta.Message)

	data, ok := resp.Data.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "not_assigned", data["status"])
}

func TestSer_LocalizesSuccessfulResponseFromAcceptLanguageHeader(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	localizer, err := i18nbuiltin.New(resti18n.LocaleENUS)
	require.NoError(t, err)

	engine := gin.New()
	engine.Use(testLanguageMiddleware(localizer, resti18n.LocaleENUS))
	engine.GET("/", func(c *gin.Context) {
		Ser(c, http.StatusOK, SuccessCodeMerchantProfileRetrieved, SuccessMessageMerchantProfileReturned, nil)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(header.AcceptLanguage, "fa-IR")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	var resp SuccessResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "fa-IR", rec.Header().Get(header.ContentLanguage))
	assert.Equal(t, "پروفایل پذیرنده دریافت شد.", resp.Meta.Message)
}

func TestSer_OmitsEmptyCorrelationID(t *testing.T) {
	t.Parallel()

	c, w := newSuccessContext("trace-1", "request-1", "")

	Ser(c, http.StatusOK, SuccessCodeHealthOK, SuccessMessageDefault, nil)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))

	meta, ok := raw["meta"].(map[string]any)
	require.True(t, ok)
	_, hasCorrelation := meta["correlation_id"]
	assert.False(t, hasCorrelation)
}

func TestSerPage_IncludesPagePagination(t *testing.T) {
	t.Parallel()

	c, w := newSuccessContext("trace-page", "request-page", "correlation-page")

	page := corepagination.OffsetPage[string]{
		Data: []string{"a", "b"},
		Meta: corepagination.PageMeta{
			Page:        2,
			Limit:       10,
			Total:       25,
			TotalPages:  3,
			HasNext:     true,
			HasPrevious: true,
		},
	}

	SerPage(c, http.StatusOK, SuccessCodeInstallmentsListed, SuccessMessageContractInstallmentsListed, page)

	var resp SuccessResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	require.NotNil(t, resp.Meta.Pagination)
	assert.Equal(t, "page", resp.Meta.Pagination.Type)
	assert.Equal(t, 2, resp.Meta.Pagination.Returned)
	assert.True(t, resp.Meta.Pagination.HasNext)
	assert.True(t, resp.Meta.Pagination.HasPrevious)
	require.NotNil(t, resp.Meta.Pagination.Page)
	assert.Equal(t, 2, resp.Meta.Pagination.Page.CurrentPage)
	assert.Equal(t, 10, resp.Meta.Pagination.Page.PageSize)
	assert.Equal(t, int64(25), resp.Meta.Pagination.Page.TotalItems)
	assert.Equal(t, 3, resp.Meta.Pagination.Page.TotalPages)
	assert.Equal(t, "trace-page", resp.Meta.TraceID)
	assert.Equal(t, "request-page", resp.Meta.RequestID)
	require.NotNil(t, resp.Meta.CorrelationID)
	assert.Equal(t, "correlation-page", *resp.Meta.CorrelationID)
}

func TestSerCursor_IncludesCursorPagination(t *testing.T) {
	t.Parallel()

	c, w := newSuccessContext("trace-cursor", "request-cursor", "correlation-cursor")

	nextCursor := "opaque-cursor-token"
	page := corepagination.CursorPage[int]{
		Data: []int{1, 2, 3},
		Meta: corepagination.CursorMeta{
			Limit:      20,
			NextCursor: &nextCursor,
			HasNext:    true,
		},
	}

	SerCursor(c, http.StatusOK, SuccessCodeInstallmentsListed, SuccessMessageContractInstallmentsListed, page)

	var resp SuccessResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	require.NotNil(t, resp.Meta.Pagination)
	assert.Equal(t, "cursor", resp.Meta.Pagination.Type)
	assert.Equal(t, 3, resp.Meta.Pagination.Returned)
	assert.True(t, resp.Meta.Pagination.HasNext)
	require.NotNil(t, resp.Meta.Pagination.Cursor)
	assert.Equal(t, 20, resp.Meta.Pagination.Cursor.Limit)
	require.NotNil(t, resp.Meta.Pagination.Cursor.NextCursor)
	assert.Equal(t, nextCursor, *resp.Meta.Pagination.Cursor.NextCursor)
	assert.Equal(t, "trace-cursor", resp.Meta.TraceID)
}

func TestSerKeyset_IncludesKeysetPagination(t *testing.T) {
	t.Parallel()

	c, w := newSuccessContext("trace-keyset", "request-keyset", "correlation-keyset")

	nextAfterID := "item-42"
	nextAfterCreatedAt := "2024-06-01T10:00:00Z"
	page := corepagination.KeysetPage[string]{
		Data: []string{"x", "y"},
		Meta: corepagination.KeysetMeta{
			Limit:              15,
			NextAfterID:        &nextAfterID,
			NextAfterCreatedAt: &nextAfterCreatedAt,
			HasNext:            true,
		},
	}

	SerKeyset(c, http.StatusOK, SuccessCodeInstallmentsListed, SuccessMessageContractInstallmentsListed, page)

	var resp SuccessResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	require.NotNil(t, resp.Meta.Pagination)
	assert.Equal(t, "keyset", resp.Meta.Pagination.Type)
	assert.Equal(t, 2, resp.Meta.Pagination.Returned)
	assert.True(t, resp.Meta.Pagination.HasNext)
	require.NotNil(t, resp.Meta.Pagination.Keyset)
	assert.Equal(t, 15, resp.Meta.Pagination.Keyset.Limit)
	require.NotNil(t, resp.Meta.Pagination.Keyset.NextAfterID)
	assert.Equal(t, nextAfterID, *resp.Meta.Pagination.Keyset.NextAfterID)
	require.NotNil(t, resp.Meta.Pagination.Keyset.NextAfterCreatedAt)
	assert.Equal(t, nextAfterCreatedAt, *resp.Meta.Pagination.Keyset.NextAfterCreatedAt)
	assert.Equal(t, "trace-keyset", resp.Meta.TraceID)
}

func newSuccessContext(traceID, requestID, correlationID string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	ctx := withTestLocalization(context.Background(), resti18n.LocaleENUS)
	ctx = trace.WithTraceID(ctx, traceID)
	ctx = trace.WithRequestID(ctx, requestID)
	if correlationID != "" {
		ctx = trace.WithCorrelationID(ctx, correlationID)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	c.Request = req

	return c, w
}

func testLanguageMiddleware(localizer resti18n.Localizer, defaultLocale resti18n.Locale) gin.HandlerFunc {
	return func(c *gin.Context) {
		locale := resti18n.ResolveLocale(
			c.GetHeader(header.AcceptLanguage),
			c.GetHeader(header.Language),
			defaultLocale,
		)

		ctx := resti18n.WithRequestState(c.Request.Context(), locale, localizer)
		c.Request = c.Request.WithContext(ctx)
		c.Header(header.ContentLanguage, locale.String())
		c.Next()
	}
}
