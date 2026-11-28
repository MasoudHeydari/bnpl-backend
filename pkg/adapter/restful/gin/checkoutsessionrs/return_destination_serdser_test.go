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

			merchantID := model.ExternalMerchantID(uuid.New())
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
			c.Request = req.WithContext(merchantterminal.WithMerchantPrincipal(req.Context(), merchantterminal.MerchantPrincipal{
				ExternalMerchantID: merchantID,
			}))
			c.Params = gin.Params{{Key: "destination_name", Value: "damz-web"}}

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
