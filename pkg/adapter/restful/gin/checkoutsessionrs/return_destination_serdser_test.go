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
	returndestination "github.com/parstis/bnpl-backend/pkg/core/policy/returndestination"
)

func TestToUpsertReturnDestinationInput_PreservesRawReturnURLWhitespace(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	cases := []string{
		" https://example.com/return",
		"https://example.com/return ",
		"\thttps://example.com/return",
		"https://example.com/return\n",
	}

	for _, rawURL := range cases {
		t.Run(rawURL, func(t *testing.T) {
			t.Parallel()

			merchantID := model.SubjectID(uuid.New())
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			payload, err := json.Marshal(map[string]string{"return_url": rawURL})
			require.NoError(t, err)

			req := httptest.NewRequest(
				http.MethodPut,
				"/api/v1/partner/return-destinations/damz-web",
				bytes.NewReader(payload),
			)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set(header.AuthSubjectID, merchantID.String())
			req.Header.Set(header.AuthTenantID, uuid.New().String())
			req.Header.Set(header.AuthContextID, uuid.New().String())
			req.Header.Set(header.AuthPermissions, permission.CheckoutReturnDestinationsManageMerchant.Code)
			c.Request = req
			c.Params = gin.Params{{Key: "destination_name", Value: "damz-web"}}

			middleware.AuthContext().Attach()(c)
			require.False(t, c.IsAborted())

			input, ok := toUpsertReturnDestinationInput(c)
			require.True(t, ok)
			require.Equal(t, rawURL, input.ReturnURL, "REST must not TrimSpace return_url")

			// Same raw value must be rejected by Core policy (UseCase path).
			require.ErrorIs(
				t,
				returndestination.Development().ValidateURL(input.ReturnURL),
				returndestination.ErrReturnURLInvalid,
			)
		})
	}
}

func TestDestinationNameParam_RejectsWithoutNormalization(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	for _, name := range []string{"Damz-Web", "damz_web", "DAMZ", " damz-web"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/partner/return-destinations/x", nil)
			c.Params = gin.Params{{Key: "destination_name", Value: name}}

			got, ok := destinationNameParam(c)
			require.False(t, ok)
			require.Empty(t, got)
		})
	}
}
