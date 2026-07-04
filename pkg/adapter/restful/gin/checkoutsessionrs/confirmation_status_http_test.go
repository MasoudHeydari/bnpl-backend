package checkoutsessionrs

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/middleware"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/checkoutsessionuc"
)

func TestGetHostedConfirmationStatus_HTTPEnvelope(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	token, err := model.ParseCheckoutSessionToken(
		"cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	)
	require.NoError(t, err)

	retryAfter := 3
	engine := gin.New()
	group := engine.Group("/api/v1/checkout-sessions")
	group.Use(middleware.HostedExternalHardening())
	group.GET("/:token/confirmation-status", func(c *gin.Context) {
		parsed, ok := checkoutTokenParam(c)
		if !ok {
			return
		}

		require.Equal(t, token, parsed)

		output := checkoutsessionuc.GetExternalConfirmationStatusOutput{
			Token:              token,
			Status:             model.ProcessingCheckoutSessionStatus,
			ConfirmationStatus: checkoutsessionuc.ConfirmationStatusProcessing,
			NextAction:         checkoutsessionuc.ConfirmationNextActionWait,
			RetryAfterSeconds:  &retryAfter,
		}
		serdser.Ser(
			c,
			http.StatusOK,
			serdser.SuccessCodeCheckoutConfirmationStatusRetrieved,
			serdser.SuccessMessageCheckoutConfirmationStatusRetrieved,
			toHostedConfirmationStatusResponse(output),
		)
	})

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/checkout-sessions/"+token.String()+"/confirmation-status",
		nil,
	)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))

	var envelope map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	data, ok := envelope["data"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, token.String(), data["checkout_session_id"])
	require.Equal(t, "processing", data["status"])
	require.Equal(t, "processing", data["confirmation_status"])
	require.Equal(t, "wait", data["next_action"])

	body := strings.ToLower(rec.Body.String())
	for _, forbidden := range []string{
		"payment_url",
		"\"contract_id\"",
		"saga_id",
		"confirmation_attempt",
		"financial_payment",
		"\"otp\"",
		"phone",
		"\"customer_id\"",
	} {
		require.NotContains(t, body, forbidden)
	}
}

func TestGetHostedConfirmationStatus_RejectsMalformedToken(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	RegisterCustomerToken(engine.Group("/api/v1"), &checkoutsessionuc.UseCase{}, RegisterOptions{})

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/checkout-sessions/not-a-token/confirmation-status",
		nil,
	)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
}
