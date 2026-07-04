package shortlinkrs_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	adapterclock "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/identifier/uuidgen"
	metricsnoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/metrics/noop"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/shortlinkrs"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy/shortlinkdestination"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/shortlinkuc"
)

func TestRegister_DynamicCodeRedirectOnPublicHost(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	pool := mock.NewPool(t)
	shortLinkRepo := mock.NewShortLink(t)
	shortLinkConnQ := mock.NewShortLinkConnQueryer(t)
	code := "aB72xQ"
	destination := "https://app.example/repayments"

	pool.EXPECT().Conn(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(ctx context.Context, fn repo.ConnHandler) error {
			return fn(ctx, nil)
		})
	shortLinkRepo.EXPECT().Conn(testifymock.Anything).Return(shortLinkConnQ)
	shortLinkConnQ.EXPECT().
		GetByCode(testifymock.Anything, code).
		Return(model.ShortLink{
			ID:             model.ShortLinkID(uuid.New()),
			Code:           code,
			DestinationURL: destination,
			CreatedAt:      time.Now().UTC(),
			UpdatedAt:      time.Now().UTC(),
		}, nil)

	destinationPolicy, err := shortlinkdestination.New([]string{"app.example"})
	require.NoError(t, err)

	uc, err := shortlinkuc.New(
		pool,
		shortLinkRepo,
		destinationPolicy,
		adapterclock.NewFixed(time.Now().UTC(), time.UTC),
		uuidgen.New(),
		metricsnoop.New(),
	)
	require.NoError(t, err)

	engine := gin.New()
	shortlinkrs.Register(engine, uc, shortlinkrs.RegisterOptions{
		PublicHost:   "dpay.ir",
		DynamicCodes: true,
	})

	req := httptest.NewRequest(http.MethodGet, "/"+code, nil)
	req.Host = "dpay.ir"
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	require.Equal(t, http.StatusFound, rec.Code)
	require.Equal(t, destination, rec.Header().Get("Location"))
	require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	require.Equal(t, "no-cache", rec.Header().Get("Pragma"))
	require.Equal(t, "no-referrer", rec.Header().Get("Referrer-Policy"))
}

func TestRegister_DynamicCodeIgnoredOnAPIHost(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	destinationPolicy, err := shortlinkdestination.New([]string{"app.example"})
	require.NoError(t, err)

	uc, err := shortlinkuc.New(
		mock.NewPool(t),
		mock.NewShortLink(t),
		destinationPolicy,
		adapterclock.NewFixed(time.Now().UTC(), time.UTC),
		uuidgen.New(),
		metricsnoop.New(),
	)
	require.NoError(t, err)

	engine := gin.New()
	engine.GET("/api/v1/health", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	shortlinkrs.Register(engine, uc, shortlinkrs.RegisterOptions{
		PublicHost:   "dpay.ir",
		DynamicCodes: true,
	})

	req := httptest.NewRequest(http.MethodGet, "/aB72xQ", nil)
	req.Host = "api.bnpl.example"
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)

	apiReq := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	apiReq.Host = "api.bnpl.example"
	apiRec := httptest.NewRecorder()
	engine.ServeHTTP(apiRec, apiReq)
	require.Equal(t, http.StatusOK, apiRec.Code)
}

func TestRegister_WithoutPublicHostDoesNotMountDynamicRoute(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	destinationPolicy, err := shortlinkdestination.New([]string{"app.example"})
	require.NoError(t, err)

	uc, err := shortlinkuc.New(
		mock.NewPool(t),
		mock.NewShortLink(t),
		destinationPolicy,
		adapterclock.NewFixed(time.Now().UTC(), time.UTC),
		uuidgen.New(),
		metricsnoop.New(),
	)
	require.NoError(t, err)

	engine := gin.New()
	engine.GET("/api/v1/health", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	shortlinkrs.Register(engine, uc, shortlinkrs.RegisterOptions{
		PublicHost:   "",
		DynamicCodes: true,
	})

	req := httptest.NewRequest(http.MethodGet, "/aB72xQ", nil)
	req.Host = "dpay.ir"
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)

	apiReq := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	apiReq.Host = "api.bnpl.example"
	apiRec := httptest.NewRecorder()
	engine.ServeHTTP(apiRec, apiReq)
	require.Equal(t, http.StatusOK, apiRec.Code)
}

func TestResolveHandler_UnknownCode(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	pool := mock.NewPool(t)
	shortLinkRepo := mock.NewShortLink(t)
	shortLinkConnQ := mock.NewShortLinkConnQueryer(t)

	pool.EXPECT().Conn(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(ctx context.Context, fn repo.ConnHandler) error {
			return fn(ctx, nil)
		})
	shortLinkRepo.EXPECT().Conn(testifymock.Anything).Return(shortLinkConnQ)
	shortLinkConnQ.EXPECT().
		GetByCode(testifymock.Anything, "missing1").
		Return(model.ShortLink{}, cerr.ResourceNotFound(nil))

	destinationPolicy, err := shortlinkdestination.New([]string{"app.example"})
	require.NoError(t, err)

	uc, err := shortlinkuc.New(
		pool,
		shortLinkRepo,
		destinationPolicy,
		adapterclock.NewFixed(time.Now().UTC(), time.UTC),
		uuidgen.New(),
		metricsnoop.New(),
	)
	require.NoError(t, err)

	engine := gin.New()
	engine.GET("/:code", shortlinkrs.ResolveHandler(uc))

	req := httptest.NewRequest(http.MethodGet, "/missing1", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	require.Equal(t, "no-cache", rec.Header().Get("Pragma"))
	require.Equal(t, "no-referrer", rec.Header().Get("Referrer-Policy"))
}
