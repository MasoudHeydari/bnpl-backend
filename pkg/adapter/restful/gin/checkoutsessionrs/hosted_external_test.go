package checkoutsessionrs

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/checkoutsessionuc"
)

const hostedExternalTestToken = "cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestRegisterHostedExternal_RoutesAreRegisteredWithoutAuthMiddleware(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	engine := gin.New()
	RegisterHostedExternal(engine.Group("/api/v1/hosted"), &checkoutsessionuc.UseCase{}, RegisterOptions{})

	routes := map[string]struct{}{}
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}

	for _, want := range []string{
		"GET /api/v1/hosted/checkout-sessions/:checkout_session_id",
		"GET /api/v1/hosted/checkout-sessions/:checkout_session_id/return",
		"POST /api/v1/hosted/checkout-sessions/:checkout_session_id/start",
		"POST /api/v1/hosted/checkout-sessions/:checkout_session_id/confirm",
	} {
		require.Contains(t, routes, want)
	}

	// A request with no gateway headers reaches the handler: a malformed token
	// is a validation error, not an authentication failure.
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/hosted/checkout-sessions/not-a-token", nil)
	engine.ServeHTTP(recorder, request)

	require.NotEqual(t, http.StatusUnauthorized, recorder.Code)
	require.NotEqual(t, http.StatusForbidden, recorder.Code)
	require.Equal(t, http.StatusUnprocessableEntity, recorder.Code)
	require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
	require.Equal(t, "no-referrer", recorder.Header().Get("Referrer-Policy"))
}

// ReturnHostedExternal never reads request query parameters (for example ?url=).
// Destination authority is only the ReturnURL persisted at checkout creation;
// see checkoutsessionuc.ReturnExternal tests for redirect composition.
func TestRegisterHostedExternal_ReturnRouteIsRegistered(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	engine := gin.New()
	RegisterHostedExternal(engine.Group("/api/v1/hosted"), &checkoutsessionuc.UseCase{}, RegisterOptions{})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/hosted/checkout-sessions/not-a-token/return?url=https://evil.example",
		nil,
	)
	engine.ServeHTTP(recorder, request)

	require.NotEqual(t, http.StatusFound, recorder.Code)
	require.Equal(t, http.StatusUnprocessableEntity, recorder.Code)
}

func TestRegisterHostedExternal_ConfirmRejectsMissingOTPBeforeUseCase(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	engine := gin.New()
	RegisterHostedExternal(engine.Group("/api/v1/hosted"), &checkoutsessionuc.UseCase{}, RegisterOptions{})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/hosted/checkout-sessions/"+hostedExternalTestToken+"/confirm",
		strings.NewReader(`{"evaluation_generation":1}`),
	)
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusUnprocessableEntity, recorder.Code)
}

func TestToHostedExternalGetResponse_UsesPublicTokenAndMaskedPhone(t *testing.T) {
	t.Parallel()

	expiresAt := time.Date(2026, time.August, 9, 12, 30, 0, 0, time.UTC)

	resp := toHostedExternalGetResponse(checkoutsessionuc.GetExternalOutput{
		Token:                model.CheckoutSessionToken(hostedExternalTestToken),
		Status:               model.OpenCheckoutSessionStatus,
		MerchantDisplayName:  "Shop",
		AmountMinor:          1000,
		Currency:             model.IRRCurrency,
		EvaluationGeneration: 2,
		Verification: checkoutsessionuc.ExternalVerification{
			Type:        checkoutsessionuc.VerificationTypeSMSOTP,
			Destination: "0912***4567",
		},
		ExpiresAt: expiresAt,
	})

	require.Equal(t, hostedExternalTestToken, resp.CheckoutSessionID)
	require.Equal(t, "Shop", resp.Merchant.DisplayName)
	require.Equal(t, 2, resp.EvaluationGeneration)
	require.Equal(t, "sms_otp", resp.Verification.Type)
	require.Equal(t, "0912***4567", resp.Verification.Destination)
	require.Equal(t, expiresAt, resp.ExpiresAt)
}

func TestToHostedExternalGetResponse_IncludesPreparedOffer(t *testing.T) {
	t.Parallel()

	offerExpires := time.Date(2026, time.August, 9, 13, 0, 0, 0, time.UTC)
	resp := toHostedExternalGetResponse(checkoutsessionuc.GetExternalOutput{
		Token:                model.CheckoutSessionToken(hostedExternalTestToken),
		Status:               model.OpenCheckoutSessionStatus,
		MerchantDisplayName:  "Shop",
		AmountMinor:          1000,
		Currency:             model.IRRCurrency,
		EvaluationGeneration: 1,
		Offer: &checkoutsessionuc.Offer{
			Outcome:              checkoutsessionuc.ApprovedOfferOutcome,
			AvailableCreditMinor: 9_999_999_999,
			Currency:             model.IRRCurrency,
			OfferExpiresAt:       &offerExpires,
		},
	})

	raw, err := json.Marshal(resp)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "available_credit_minor")
	require.Contains(t, string(raw), `"evaluation_generation":1`)
	require.Contains(t, string(raw), `"outcome":"approved"`)
}

func TestToHostedExternalStartResponse_OmitsOfferPayload(t *testing.T) {
	t.Parallel()

	expiresAt := time.Date(2026, time.August, 9, 12, 2, 0, 0, time.UTC)

	resp := toHostedExternalStartResponse(checkoutsessionuc.StartExternalOutput{
		Session: model.CheckoutSession{
			ID:                   model.CheckoutSessionID(uuid.New()),
			PublicToken:          model.CheckoutSessionToken(hostedExternalTestToken),
			Status:               model.OpenCheckoutSessionStatus,
			EvaluationGeneration: 1,
		},
		EvaluationGeneration: 1,
		OTPSent:              true,
		OTPExpiresAt:         &expiresAt,
	})

	raw, err := json.Marshal(resp)
	require.NoError(t, err)
	require.NotContains(t, string(raw), `"offer"`)
	require.NotContains(t, string(raw), "available_credit_minor")
}

func TestToHostedExternalStartResponse_AddressesCheckoutByPublicToken(t *testing.T) {
	t.Parallel()

	expiresAt := time.Date(2026, time.August, 9, 12, 2, 0, 0, time.UTC)

	resp := toHostedExternalStartResponse(checkoutsessionuc.StartExternalOutput{
		Session: model.CheckoutSession{
			ID:                   model.CheckoutSessionID(uuid.New()),
			PublicToken:          model.CheckoutSessionToken(hostedExternalTestToken),
			Status:               model.OpenCheckoutSessionStatus,
			EvaluationGeneration: 3,
		},
		EvaluationGeneration: 3,
		OTPSent:              true,
		OTPExpiresAt:         &expiresAt,
	})

	require.Equal(t, hostedExternalTestToken, resp.CheckoutSessionID)
	require.Equal(t, 3, resp.EvaluationGeneration)
	require.True(t, resp.OTPSent)
	require.Equal(t, &expiresAt, resp.OTPExpiresAt)
}

func TestToHostedExternalConfirmResponse_AddressesCheckoutByPublicToken(t *testing.T) {
	t.Parallel()

	resp := toHostedExternalConfirmResponse(checkoutsessionuc.ConfirmOutput{
		ReconfirmRequired: true,
		Session: model.CheckoutSession{
			ID:                   model.CheckoutSessionID(uuid.New()),
			PublicToken:          model.CheckoutSessionToken(hostedExternalTestToken),
			Status:               model.OpenCheckoutSessionStatus,
			EvaluationGeneration: 2,
		},
		Offer: checkoutsessionuc.Offer{Outcome: checkoutsessionuc.DeniedOfferOutcome},
	})

	require.Equal(t, hostedExternalTestToken, resp.CheckoutSessionID)
	require.True(t, resp.ReconfirmRequired)
	require.Equal(t, 2, resp.EvaluationGeneration)
	require.NotNil(t, resp.Offer)
	require.Nil(t, resp.Contract)
}
