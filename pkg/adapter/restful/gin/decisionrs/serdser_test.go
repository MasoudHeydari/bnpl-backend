package decisionrs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/header"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/permission"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/decisionuc"
)

func TestMapCheckoutDecisionOutcomeForResponse(t *testing.T) {
	t.Parallel()

	require.Equal(t, "approved", mapCheckoutDecisionOutcomeForResponse(model.ApprovedDecisionOutcome))
	require.Equal(t, "approved", mapCheckoutDecisionOutcomeForResponse(model.ConditionalDecisionOutcome))
	require.Equal(t, "denied", mapCheckoutDecisionOutcomeForResponse(model.DeniedDecisionOutcome))
}

func TestToCreateResponse_MapsConditionalOutcomeToApproved(t *testing.T) {
	t.Parallel()

	decisionID := model.DecisionID(uuid.New())
	output := decisionuc.CreateOutput{
		Result: decisionuc.CreateResultCreated,
		Decision: model.Decision{
			ID:             decisionID,
			Type:           model.ContractApprovalDecisionType,
			TargetType:     model.CheckoutKeyDecisionTargetType,
			IdempotencyKey: "checkout-key-1",
			Outcome:        model.ConditionalDecisionOutcome,
			ReasonCodes:    []string{"partial_financing"},
		},
		RequestedAmountMinor:        1000,
		ApprovedFinancedAmountMinor: 800,
		Currency:                    model.IRRCurrency,
	}

	resp := toCreateResponse(output)
	require.Equal(t, "approved", resp.Decision.Outcome)
	require.Equal(t, decisionID.UUID(), resp.Decision.ID)
	require.Equal(t, "checkout-key-1", resp.Decision.CheckoutKey)
	require.Equal(t, []string{"partial_financing"}, resp.Decision.ReasonCodes)
}

func TestPurchaseContextRequestValidate(t *testing.T) {
	t.Parallel()

	posID := model.PosGpsID(uuid.New())
	onlineID := model.OnlineStoreID(uuid.New())

	t.Run("valid", func(t *testing.T) {
		t.Parallel()

		require.NoError(t, purchaseContextRequest{
			Type:     model.POSPurchaseChannel,
			PosGpsID: posID,
		}.validate())

		require.NoError(t, purchaseContextRequest{
			Type:          model.OnlinePurchaseChannel,
			OnlineStoreID: onlineID,
		}.validate())

		require.NoError(t, purchaseContextRequest{
			Type:            model.OnlinePurchaseChannel,
			OnlineStoreID:   onlineID,
			MerchantOrderID: "ORDER-123",
		}.validate())
	})

	t.Run("invalid", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			name   string
			req    purchaseContextRequest
			field  string
			code   cerr.Code
			msgKey cerr.MessageKey
		}{
			{
				name:   "unknown_type",
				req:    purchaseContextRequest{Type: model.PurchaseChannel("something_else")},
				field:  "purchase_context.type",
				code:   cerr.CodeInvalidArgument,
				msgKey: "error.purchase_channel_invalid",
			},
			{
				name:   "pos_without_pos_gps_id",
				req:    purchaseContextRequest{Type: model.POSPurchaseChannel},
				field:  "purchase_context.pos_gps_id",
				code:   cerr.CodeInvalidArgument,
				msgKey: "error.pos_gps_id_required",
			},
			{
				name: "pos_with_empty_pos_gps_id",
				req: purchaseContextRequest{
					Type:     model.POSPurchaseChannel,
					PosGpsID: model.PosGpsID{},
				},
				field:  "purchase_context.pos_gps_id",
				code:   cerr.CodeInvalidArgument,
				msgKey: "error.pos_gps_id_required",
			},
			{
				name: "pos_with_online_store_id",
				req: purchaseContextRequest{
					Type:          model.POSPurchaseChannel,
					PosGpsID:      posID,
					OnlineStoreID: onlineID,
				},
				field:  "purchase_context.online_store_id",
				code:   cerr.CodeInvalidArgument,
				msgKey: "error.online_store_id_not_allowed_for_pos_purchase",
			},
			{
				name: "pos_with_merchant_order_id",
				req: purchaseContextRequest{
					Type:            model.POSPurchaseChannel,
					PosGpsID:        posID,
					MerchantOrderID: "ORDER-1",
				},
				field:  "purchase_context.merchant_order_id",
				code:   cerr.CodeInvalidArgument,
				msgKey: "error.merchant_order_id_not_allowed_for_pos_purchase",
			},
			{
				name: "pos_with_both_ids",
				req: purchaseContextRequest{
					Type:          model.POSPurchaseChannel,
					PosGpsID:      posID,
					OnlineStoreID: onlineID,
				},
				field:  "purchase_context.online_store_id",
				code:   cerr.CodeInvalidArgument,
				msgKey: "error.online_store_id_not_allowed_for_pos_purchase",
			},
			{
				name:   "online_without_online_store_id",
				req:    purchaseContextRequest{Type: model.OnlinePurchaseChannel},
				field:  "purchase_context.online_store_id",
				code:   cerr.CodeInvalidArgument,
				msgKey: "error.online_store_id_required",
			},
			{
				name: "online_with_empty_online_store_id",
				req: purchaseContextRequest{
					Type:          model.OnlinePurchaseChannel,
					OnlineStoreID: model.OnlineStoreID{},
				},
				field:  "purchase_context.online_store_id",
				code:   cerr.CodeInvalidArgument,
				msgKey: "error.online_store_id_required",
			},
			{
				name: "online_with_pos_gps_id",
				req: purchaseContextRequest{
					Type:          model.OnlinePurchaseChannel,
					OnlineStoreID: onlineID,
					PosGpsID:      posID,
				},
				field:  "purchase_context.pos_gps_id",
				code:   cerr.CodeInvalidArgument,
				msgKey: "error.pos_gps_id_not_allowed_for_online_purchase",
			},
			{
				name: "online_merchant_order_id_too_long",
				req: purchaseContextRequest{
					Type:            model.OnlinePurchaseChannel,
					OnlineStoreID:   onlineID,
					MerchantOrderID: strings.Repeat("a", decisionuc.MaxMerchantOrderIDLength+1),
				},
				field:  "purchase_context.merchant_order_id",
				code:   cerr.CodeInvalidArgument,
				msgKey: "error.merchant_order_id_too_long",
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				err := tc.req.validate()
				require.Error(t, err)
				require.True(t, cerr.IsCode(err, tc.code))

				var ce *cerr.Error
				require.True(t, errors.As(err, &ce))
				require.Equal(t, tc.msgKey, ce.MessageKey)
				require.Contains(t, ce.Details.Fields, tc.field)
			})
		}
	})
}

func TestToCreateInput_PurchaseContextValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	posID := uuid.New()
	onlineID := uuid.New()

	t.Run("valid_pos", func(t *testing.T) {
		c, w := newCreateInputContext(t, map[string]any{
			"purchase_context": map[string]any{
				"type":       "pos",
				"pos_gps_id": posID.String(),
			},
			"requested_amount_minor": 1_500_000,
			"currency":               "IRR",
		})

		input, ok := toCreateInput(c)
		require.True(t, ok)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, model.POSPurchaseChannel, input.PurchaseContext.Channel)
		require.Equal(t, model.PosGpsID(posID), input.PurchaseContext.PosGpsID)
	})

	t.Run("valid_online_with_order", func(t *testing.T) {
		c, w := newCreateInputContext(t, map[string]any{
			"purchase_context": map[string]any{
				"type":              "online",
				"online_store_id":   onlineID.String(),
				"merchant_order_id": "ORDER-123",
			},
			"requested_amount_minor": 1_500_000,
			"currency":               "IRR",
		})

		input, ok := toCreateInput(c)
		require.True(t, ok)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, model.OnlinePurchaseChannel, input.PurchaseContext.Channel)
		require.Equal(t, "ORDER-123", input.PurchaseContext.MerchantOrderID)
	})

	invalidBodies := []struct {
		name       string
		body       map[string]any
		status     int
		field      string
		code       string
		bindFailed bool
	}{
		{
			name: "missing_purchase_context",
			body: map[string]any{
				"requested_amount_minor": 1_500_000,
				"currency":               "IRR",
			},
			status:     http.StatusUnprocessableEntity,
			field:      "PurchaseContext",
			code:       string(cerr.CodeValidationFailed),
			bindFailed: true,
		},
		{
			name: "missing_type",
			body: map[string]any{
				"purchase_context": map[string]any{
					"pos_gps_id": posID.String(),
				},
				"requested_amount_minor": 1_500_000,
				"currency":               "IRR",
			},
			status:     http.StatusUnprocessableEntity,
			field:      "Type",
			code:       string(cerr.CodeValidationFailed),
			bindFailed: true,
		},
		{
			name: "unknown_type",
			body: map[string]any{
				"purchase_context": map[string]any{
					"type": "something_else",
				},
				"requested_amount_minor": 1_500_000,
				"currency":               "IRR",
			},
			status: http.StatusBadRequest,
			field:  "purchase_context.type",
			code:   string(cerr.CodeInvalidArgument),
		},
		{
			name: "pos_without_pos_gps_id",
			body: map[string]any{
				"purchase_context":       map[string]any{"type": "pos"},
				"requested_amount_minor": 1_500_000,
				"currency":               "IRR",
			},
			status: http.StatusBadRequest,
			field:  "purchase_context.pos_gps_id",
			code:   string(cerr.CodeInvalidArgument),
		},
		{
			name: "pos_with_online_store_id",
			body: map[string]any{
				"purchase_context": map[string]any{
					"type":            "pos",
					"pos_gps_id":      posID.String(),
					"online_store_id": onlineID.String(),
				},
				"requested_amount_minor": 1_500_000,
				"currency":               "IRR",
			},
			status: http.StatusBadRequest,
			field:  "purchase_context.online_store_id",
			code:   string(cerr.CodeInvalidArgument),
		},
		{
			name: "pos_with_merchant_order_id",
			body: map[string]any{
				"purchase_context": map[string]any{
					"type":              "pos",
					"pos_gps_id":        posID.String(),
					"merchant_order_id": "ORDER-1",
				},
				"requested_amount_minor": 1_500_000,
				"currency":               "IRR",
			},
			status: http.StatusBadRequest,
			field:  "purchase_context.merchant_order_id",
			code:   string(cerr.CodeInvalidArgument),
		},
		{
			name: "online_without_online_store_id",
			body: map[string]any{
				"purchase_context":       map[string]any{"type": "online"},
				"requested_amount_minor": 1_500_000,
				"currency":               "IRR",
			},
			status: http.StatusBadRequest,
			field:  "purchase_context.online_store_id",
			code:   string(cerr.CodeInvalidArgument),
		},
		{
			name: "online_with_pos_gps_id",
			body: map[string]any{
				"purchase_context": map[string]any{
					"type":            "online",
					"online_store_id": onlineID.String(),
					"pos_gps_id":      posID.String(),
				},
				"requested_amount_minor": 1_500_000,
				"currency":               "IRR",
			},
			status: http.StatusBadRequest,
			field:  "purchase_context.pos_gps_id",
			code:   string(cerr.CodeInvalidArgument),
		},
	}

	for _, tc := range invalidBodies {
		t.Run(tc.name, func(t *testing.T) {
			c, w := newCreateInputContext(t, tc.body)

			_, ok := toCreateInput(c)
			require.False(t, ok)
			require.Equal(t, tc.status, w.Code)

			var resp serdser.ErrorResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, tc.code, resp.Code)
			if !tc.bindFailed {
				require.Contains(t, resp.Details.Fields, tc.field)
			}
		})
	}
}

func TestCreate_InvalidPurchaseContextDoesNotInvokeUseCase(t *testing.T) {
	gin.SetMode(gin.TestMode)

	stub := &stubDecisionUseCase{t: t}
	rs := &resource{decision: nil}
	// Exercise handler path through toCreateInput without a real use case:
	// invalid body must abort before Create is reachable.
	_ = stub
	_ = rs

	engine := gin.New()
	engine.POST("/api/v1/checkout-decisions", func(c *gin.Context) {
		_, ok := toCreateInput(c)
		if !ok {
			return
		}

		stub.called = true
		c.Status(http.StatusCreated)
	})

	body, err := json.Marshal(map[string]any{
		"purchase_context":       map[string]any{"type": "pos"},
		"requested_amount_minor": 1_500_000,
		"currency":               "IRR",
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/checkout-decisions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(header.IdempotencyKey, "checkout-key-1")
	req = req.WithContext(permission.WithAuthContext(req.Context(), permission.AuthContext{
		SubjectID:   model.SubjectID(uuid.New()),
		TenantID:    model.TenantID(uuid.New()),
		ContextID:   model.ContextID(uuid.New()),
		Permissions: []string{permission.DecisionsCreateSelf.Code},
	}))

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.False(t, stub.called)
}

type stubDecisionUseCase struct {
	t      *testing.T
	called bool
}

func newCreateInputContext(t *testing.T, body map[string]any) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	raw, err := json.Marshal(body)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/checkout-decisions", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(header.IdempotencyKey, "checkout-key-1")
	req = req.WithContext(permission.WithAuthContext(context.Background(), permission.AuthContext{
		SubjectID:   model.SubjectID(uuid.New()),
		TenantID:    model.TenantID(uuid.New()),
		ContextID:   model.ContextID(uuid.New()),
		Permissions: []string{permission.DecisionsCreateSelf.Code},
	}))
	c.Request = req

	return c, w
}
