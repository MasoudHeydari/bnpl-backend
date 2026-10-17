package checkoutsessionrs

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/merchantterminal"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/permission"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/checkoutsessionuc"
)

func TestToCreatePartnerInput_CustomerPhonePassesRawUnchanged(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	for _, raw := range []string{
		"09121234567",
		"+989121234567",
		" 09121234567",
	} {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()

			input, ok := partnerCreateInputWithPhone(t, raw)
			require.True(t, ok)
			require.Equal(t, raw, input.CustomerPhone, "REST must not trim or canonicalize customer_phone")
		})
	}
}

func TestToCreatePartnerInput_CustomerPhoneRejectsOverMaxLength(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	tooLong := make([]byte, model.MaxCustomerPhoneLength+1)
	for i := range tooLong {
		tooLong[i] = '9'
	}

	_, ok := partnerCreateInputWithPhone(t, string(tooLong))
	require.False(t, ok)
}

func TestToCreatePartnerInput_IgnoresForgedGatewayAuthScopeHeaders(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	input, ok := partnerCreateInputWithPhone(t, "+989121234567")
	require.True(t, ok)
	require.False(t, input.MerchantID.IsZero())
	// CreatePartnerInput no longer carries tenant/context from headers.
}

func partnerCreateInputWithPhone(t *testing.T, customerPhone string) (checkoutsessionuc.CreatePartnerInput, bool) {
	t.Helper()

	merchantID := model.MerchantID(uuid.New())
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	payload, err := json.Marshal(map[string]any{
		"online_store_id":        uuid.New().String(),
		"merchant_order_id":      "order-1",
		"idempotency_key":        "attempt-1",
		"customer_phone":         customerPhone,
		"return_destination":     "storefront",
		"requested_amount_minor": 150000,
		"currency":               "IRR",
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/partner/checkout-sessions", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	// Forged JWT-style headers must not be required and must not be trusted.
	req.Header.Set("X-Auth-Tenant-Id", uuid.New().String())
	req.Header.Set("X-Auth-Context-Id", uuid.New().String())
	c.Request = req

	principal := merchantterminal.MerchantPrincipal{
		MerchantID:  merchantID,
		TerminalID:  4242,
		Permissions: []string{permission.CheckoutSessionsCreateMerchant.Code},
	}
	authCtx := permission.AuthContext{
		SubjectID:   model.SubjectID(merchantID),
		Permissions: principal.Permissions,
	}
	ctx := merchantterminal.WithMerchantPrincipal(c.Request.Context(), principal)
	ctx = permission.WithAuthContext(ctx, authCtx)
	c.Request = c.Request.WithContext(ctx)

	return toCreatePartnerInput(c)
}
