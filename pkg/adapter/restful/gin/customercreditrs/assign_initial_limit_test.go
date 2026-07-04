package customercreditrs_test

import (
	"context"
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
	initialcreditlimitv1 "github.com/parstis/bnpl-backend/pkg/core/policy/initialcreditlimit/v1"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/customercredituc"
)

type stubCreditLimitUseCase struct {
	lastInput customercredituc.AssignInitialLimitInput
	lastCtx   context.Context
}

func (s *stubCreditLimitUseCase) AssignInitialLimit(
	ctx context.Context,
	input customercredituc.AssignInitialLimitInput,
) (customercredituc.AssignInitialLimitOutput, error) {
	s.lastCtx = ctx
	s.lastInput = input
	return customercredituc.AssignInitialLimitOutput{
		Result: customercredituc.AssignInitialLimitResultCreated,
		Decision: model.Decision{
			ID:            model.DecisionID(uuid.New()),
			Type:          model.CreditLimitDecisionType,
			TargetType:    model.CustomerProfileDecisionTargetType,
			TargetID:      uuid.New(),
			Outcome:       model.LimitSetDecisionOutcome,
			ReasonCodes:   []string{initialcreditlimitv1.New().Terms().ApprovalReasonCode},
			PolicyID:      "customer_default_initial_limit",
			PolicyVersion: "v1",
		},
	}, nil
}

func TestAssignInitialLimit_UsesAuthSubjectAsExternalCustomerID_EmptyBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	externalCustomerID := model.ExternalCustomerID(uuid.New())
	tenantID := model.TenantID(uuid.New())
	contextID := model.ContextID(uuid.New())
	stub := &stubCreditLimitUseCase{}

	engine := gin.New()
	authMW := middleware.AuthContext()
	engine.Use(authMW.Attach())
	engine.POST("/api/v1/customer-credit/initiate", func(c *gin.Context) {
		authCtx, ok := permission.FromContext(c.Request.Context())
		require.True(t, ok)
		_, err := stub.AssignInitialLimit(c.Request.Context(), customercredituc.AssignInitialLimitInput{
			ExternalCustomerID: authCtx.SubjectID.AsExternalCustomerID(),
		})
		require.NoError(t, err)
		c.Status(http.StatusCreated)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/customer-credit/initiate", http.NoBody)
	req.Header.Set(header.AuthSubjectID, externalCustomerID.String())
	req.Header.Set(header.AuthTenantID, tenantID.String())
	req.Header.Set(header.AuthContextID, contextID.String())
	req.Header.Set(header.AuthPermissions, permission.CreditLimitsCreateSelf.Code)

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusCreated, recorder.Code)
	require.Equal(t, externalCustomerID, stub.lastInput.ExternalCustomerID)

	authCtx, ok := permission.FromContext(stub.lastCtx)
	require.True(t, ok)
	require.Equal(t, tenantID, authCtx.TenantID)
	require.Equal(t, contextID, authCtx.ContextID)
	require.Equal(t, []string{permission.CreditLimitsCreateSelf.Code}, authCtx.Permissions)
}
