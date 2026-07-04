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

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/middleware"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/header"
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

func partnerCreateInputWithPhone(t *testing.T, customerPhone string) (checkoutsessionuc.CreatePartnerInput, bool) {
	t.Helper()

	merchantID := model.SubjectID(uuid.New())
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
	req.Header.Set(header.AuthSubjectID, merchantID.String())
	req.Header.Set(header.AuthTenantID, uuid.New().String())
	req.Header.Set(header.AuthContextID, uuid.New().String())
	req.Header.Set(header.AuthPermissions, permission.CheckoutSessionsCreateMerchant.Code)
	c.Request = req

	middleware.AuthContext().Attach()(c)
	require.False(t, c.IsAborted())

	return toCreatePartnerInput(c)
}
