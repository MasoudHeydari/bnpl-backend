package healthrs

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/middleware"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/header"
	resti18n "github.com/parstis/bnpl-backend/pkg/adapter/restful/i18n"
	i18nbuiltin "github.com/parstis/bnpl-backend/pkg/adapter/restful/i18n/builtin"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/trace"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/healthcheckuc"
)

type healthDeps struct {
	lifecycleState    *mock.LifecycleState
	dependencyChecker *mock.DependencyChecker
}

func newHealthDeps(t *testing.T) healthDeps {
	t.Helper()

	return healthDeps{
		lifecycleState:    mock.NewLifecycleState(t),
		dependencyChecker: mock.NewDependencyChecker(t),
	}
}

func (d healthDeps) newUseCase() *healthcheckuc.UseCase {
	return healthcheckuc.New(d.lifecycleState, d.dependencyChecker)
}

func newHealthEngine(t *testing.T, uc *healthcheckuc.UseCase, enabled bool) *gin.Engine {
	t.Helper()

	gin.SetMode(gin.TestMode)
	e := gin.New()
	Register(e, uc, RegisterOptions{Enabled: enabled})
	return e
}

func TestLive_ReturnsUPWhenHealthy(t *testing.T) {
	t.Parallel()

	deps := newHealthDeps(t)
	deps.lifecycleState.EXPECT().IsShutdownTimedOut().Return(false).Once()

	e := newHealthEngine(t, deps.newUseCase(), true)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz/live", nil)
	e.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "UP", resp.Status)
}

func TestLive_ReturnsServiceUnavailableWhenShutdownTimedOut(t *testing.T) {
	t.Parallel()

	deps := newHealthDeps(t)
	deps.lifecycleState.EXPECT().IsShutdownTimedOut().Return(true).Once()

	e := newHealthEngine(t, deps.newUseCase(), true)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz/live", nil)
	e.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestReady_ReturnsUPWhenHealthy(t *testing.T) {
	t.Parallel()

	deps := newHealthDeps(t)
	deps.lifecycleState.EXPECT().IsDraining().Return(false).Once()
	deps.dependencyChecker.EXPECT().Check(context.Background()).Return(nil).Once()

	e := newHealthEngine(t, deps.newUseCase(), true)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz/ready", nil)
	e.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "UP", resp.Status)
}

func TestLive_KeepsCanonicalUPForPersianRequest(t *testing.T) {
	t.Parallel()

	deps := newHealthDeps(t)
	deps.lifecycleState.EXPECT().IsShutdownTimedOut().Return(false).Once()

	localizer, err := i18nbuiltin.New(resti18n.LocaleENUS)
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)
	e := gin.New()
	e.Use(middleware.Language(localizer, resti18n.LocaleENUS))
	Register(e, deps.newUseCase(), RegisterOptions{Enabled: true})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz/live", nil)
	req.Header.Set(header.Language, "fa-IR")
	e.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "UP", resp.Status)
	assert.Equal(t, "fa-IR", w.Header().Get(header.ContentLanguage))
}

func TestReady_LocalizesHumanFacingErrorForPersianRequest(t *testing.T) {
	t.Parallel()

	deps := newHealthDeps(t)
	deps.lifecycleState.EXPECT().IsDraining().Return(true).Once()

	localizer, err := i18nbuiltin.New(resti18n.LocaleENUS)
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)
	e := gin.New()
	e.Use(middleware.Language(localizer, resti18n.LocaleENUS))
	Register(e, deps.newUseCase(), RegisterOptions{Enabled: true})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz/ready", nil)
	req.Header.Set(header.Language, "fa-IR")
	e.ServeHTTP(w, req)

	require.Equal(t, http.StatusServiceUnavailable, w.Code)

	var resp serdser.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, string(cerr.CodeServiceUnavailable), resp.Code)
	assert.Equal(t, "سرویس در حال توقف و تخلیه درخواست‌ها است.", resp.Message)
	assert.Equal(t, "fa-IR", w.Header().Get(header.ContentLanguage))
}

func TestReady_ReturnsServiceUnavailableWhenDraining(t *testing.T) {
	t.Parallel()

	deps := newHealthDeps(t)
	deps.lifecycleState.EXPECT().IsDraining().Return(true).Once()

	e := newHealthEngine(t, deps.newUseCase(), true)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz/ready", nil)
	e.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestReady_ReturnsErrorWhenDependencyCheckFails(t *testing.T) {
	t.Parallel()

	deps := newHealthDeps(t)
	checkErr := cerr.ServiceUnavailable(errors.New("connection refused"))
	deps.lifecycleState.EXPECT().IsDraining().Return(false).Once()
	deps.dependencyChecker.EXPECT().Check(context.Background()).Return(checkErr).Once()

	e := newHealthEngine(t, deps.newUseCase(), true)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz/ready", nil)
	e.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestRegister_DisabledDoesNotRegisterRoutes(t *testing.T) {
	t.Parallel()

	deps := newHealthDeps(t)
	e := newHealthEngine(t, deps.newUseCase(), false)

	for _, path := range []string{"/healthz/live", "/healthz/ready"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		e.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code, "path %s should not be registered", path)
	}
}

func TestReady_ErrorResponseIncludesTraceID(t *testing.T) {
	t.Parallel()

	deps := newHealthDeps(t)
	deps.lifecycleState.EXPECT().IsDraining().Return(true).Once()

	e := newHealthEngine(t, deps.newUseCase(), true)
	w := httptest.NewRecorder()

	ctx := trace.WithTraceID(context.Background(), "health-trace")
	req := httptest.NewRequest(http.MethodGet, "/healthz/ready", nil).WithContext(ctx)
	e.ServeHTTP(w, req)

	var resp struct {
		TraceID string `json:"trace_id"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "health-trace", resp.TraceID)
}
