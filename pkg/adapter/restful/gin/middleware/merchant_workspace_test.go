package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/header"
)

func TestMerchantWorkspaceFromHeaders(t *testing.T) {
	const merchantID = "88888888-8888-4888-8888-888888888888"
	const outletID = "99999999-9999-4999-8999-999999999999"

	for _, tc := range []struct {
		name  string
		scope string
		ids   string
		valid bool
	}{
		{"all", "ALL_OUTLETS", "[]", true},
		{"selected", "SELECTED_OUTLETS", `["` + outletID + `"]`, true},
		{"empty selected", "SELECTED_OUTLETS", "[]", true},
		{"all with ids", "ALL_OUTLETS", `["` + outletID + `"]`, false},
		{"unknown", "UNKNOWN", "[]", false},
		{"missing", "SELECTED_OUTLETS", "", false},
		{"null", "SELECTED_OUTLETS", "null", false},
		{"duplicate", "SELECTED_OUTLETS", `["` + outletID + `","` + outletID + `"]`, false},
		{"malformed", "SELECTED_OUTLETS", `["not-a-uuid"]`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("GET", "/", nil)
			c.Request.Header.Set(header.AuthMerchantID, merchantID)
			c.Request.Header.Set(header.AuthOutletScope, tc.scope)
			c.Request.Header.Set(header.AuthOutletIDs, tc.ids)
			workspace, err := merchantWorkspaceFromHeaders(c)
			if !tc.valid {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, merchantID, workspace.MerchantID.String())
			require.Equal(t, tc.scope == "ALL_OUTLETS", workspace.Scope.IsAllOutlets())
		})
	}
}
