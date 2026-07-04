package merchantterminalrs

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/header"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/permission"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/merchantterminaluc"
)

// cacheControlNoStore keeps responses that carry a plaintext API key out of
// shared and browser caches.
const cacheControlNoStore = "no-store"

// maxGraceSeconds is the largest grace period that converts to a time.Duration
// without overflow. Larger values are clamped and rejected by the use case as
// exceeding the configured maximum.
const maxGraceSeconds = int64(math.MaxInt64 / int64(time.Second))

type rotateRequest struct {
	// GracePeriodSeconds is how long the previous API key stays accepted. Omit
	// it to use the server default; 0 retires the previous key immediately.
	GracePeriodSeconds *int64 `json:"grace_period_seconds,omitempty" binding:"omitempty,min=0" example:"3600" minimum:"0"`
}

type provisionResponse struct {
	MerchantID        model.MerchantID                   `json:"merchant_id" example:"8e1a2b3c-4d5e-4f6a-8b7c-9d0e1f2a3b4c" format:"uuid"`
	TerminalID        model.TerminalID                   `json:"terminal_id" swaggertype:"string" example:"4829174650293817"`
	CredentialID      model.MerchantTerminalCredentialID `json:"credential_id" example:"7f3d9a10-2b4c-4e5f-9a6b-1c2d3e4f5a6b" format:"uuid"`
	CredentialVersion int64                              `json:"credential_version" example:"1"`
	APIKey            string                             `json:"api_key" example:"bnpl_live_example"`
	CreatedAt         time.Time                          `json:"created_at" example:"2026-08-09T12:00:00Z"`
	ExpiresAt         time.Time                          `json:"api_key_replayable_until" example:"2026-08-10T12:00:00Z"`
	Replayed          bool                               `json:"replayed" example:"false"`
}

type retiringCredentialResponse struct {
	CredentialID model.MerchantTerminalCredentialID `json:"credential_id" example:"7f3d9a10-2b4c-4e5f-9a6b-1c2d3e4f5a6b" format:"uuid"`
	Version      int64                              `json:"version" example:"1"`
	RetireAt     time.Time                          `json:"retire_at" example:"2026-08-10T12:00:00Z"`
}

type rotateResponse struct {
	MerchantID        model.MerchantID                   `json:"merchant_id" example:"8e1a2b3c-4d5e-4f6a-8b7c-9d0e1f2a3b4c" format:"uuid"`
	TerminalID        model.TerminalID                   `json:"terminal_id" swaggertype:"string" example:"4829174650293817"`
	CredentialID      model.MerchantTerminalCredentialID `json:"credential_id" example:"7f3d9a10-2b4c-4e5f-9a6b-1c2d3e4f5a6b" format:"uuid"`
	CredentialVersion int64                              `json:"credential_version" example:"2"`
	APIKey            string                             `json:"api_key" example:"bnpl_live_example"`
	ActivatedAt       time.Time                          `json:"activated_at" example:"2026-08-09T12:00:00Z"`
	Retiring          *retiringCredentialResponse        `json:"retiring,omitempty"`
	ExpiresAt         time.Time                          `json:"api_key_replayable_until" example:"2026-08-10T12:00:00Z"`
	Replayed          bool                               `json:"replayed" example:"false"`
}

type retireResponse struct {
	CredentialID model.MerchantTerminalCredentialID `json:"credential_id" example:"7f3d9a10-2b4c-4e5f-9a6b-1c2d3e4f5a6b" format:"uuid"`
	Version      int64                              `json:"version" example:"1"`
	RetiredAt    time.Time                          `json:"retired_at" example:"2026-08-09T12:00:00Z"`
}

type activeCredentialResponse struct {
	CredentialID model.MerchantTerminalCredentialID `json:"credential_id" example:"7f3d9a10-2b4c-4e5f-9a6b-1c2d3e4f5a6b" format:"uuid"`
	Version      int64                              `json:"version" example:"1"`
	ActivatedAt  time.Time                          `json:"activated_at" example:"2026-08-09T12:00:00Z"`
}

type getResponse struct {
	MerchantID       model.MerchantID             `json:"merchant_id" example:"8e1a2b3c-4d5e-4f6a-8b7c-9d0e1f2a3b4c" format:"uuid"`
	TerminalID       model.TerminalID             `json:"terminal_id" swaggertype:"string" example:"4829174650293817"`
	Status           model.MerchantTerminalStatus `json:"status" example:"active"`
	CreatedAt        time.Time                    `json:"created_at" example:"2026-08-09T12:00:00Z"`
	UpdatedAt        time.Time                    `json:"updated_at" example:"2026-08-09T12:00:00Z"`
	ActiveCredential *activeCredentialResponse    `json:"active_credential,omitempty"`
	Retiring         *retiringCredentialResponse  `json:"retiring,omitempty"`
}

// idempotencyKey reads the required Idempotency-Key header. Length limits are
// enforced by the use case.
func idempotencyKey(c *gin.Context) (string, bool) {
	key := strings.TrimSpace(c.GetHeader(header.IdempotencyKey))
	if key == "" {
		serdser.SerValidationErr(c, header.IdempotencyKey, "validation.idempotency_key_header_required")

		return "", false
	}

	return key, true
}

func authenticatedMerchantID(c *gin.Context) (model.MerchantID, bool) {
	const op = "merchantterminalrs.authenticatedMerchantID"

	authCtx, ok := permission.FromContext(c.Request.Context())
	if !ok {
		serdser.SerErr(
			c,
			cerr.NewPublic(
				cerr.CodeUnauthorized,
				"error.authentication_context_missing",
				fmt.Errorf("%s: authentication context is missing", op),
				cerr.Details{
					Fields: []string{
						header.AuthSubjectID,
						header.AuthTenantID,
						header.AuthContextID,
					},
					Violations: []cerr.Violation{
						{
							Code:       "REQUIRED",
							MessageKey: "error.trusted_authentication_headers_are_required",
							Fields: []string{
								header.AuthSubjectID,
								header.AuthTenantID,
								header.AuthContextID,
							},
						},
					},
				},
			),
		)

		return model.MerchantID{}, false
	}

	return authCtx.SubjectID.AsMerchantID(), true
}

func toProvisionInput(c *gin.Context) (merchantterminaluc.ProvisionInput, bool) {
	merchantID, ok := authenticatedMerchantID(c)
	if !ok {
		return merchantterminaluc.ProvisionInput{}, false
	}

	key, ok := idempotencyKey(c)
	if !ok {
		return merchantterminaluc.ProvisionInput{}, false
	}

	return merchantterminaluc.ProvisionInput{
		MerchantID:     merchantID,
		IdempotencyKey: key,
	}, true
}

func toGetInput(c *gin.Context) (merchantterminaluc.GetInput, bool) {
	merchantID, ok := authenticatedMerchantID(c)
	if !ok {
		return merchantterminaluc.GetInput{}, false
	}

	return merchantterminaluc.GetInput{MerchantID: merchantID}, true
}

func toGetResponse(output merchantterminaluc.GetOutput) []getResponse {
	items := make([]getResponse, 0, len(output.Items))

	for _, terminal := range output.Items {
		response := getResponse{
			MerchantID: terminal.MerchantID,
			TerminalID: terminal.TerminalID,
			Status:     terminal.Status,
			CreatedAt:  terminal.CreatedAt,
			UpdatedAt:  terminal.UpdatedAt,
		}

		if terminal.ActiveCredential != nil {
			response.ActiveCredential = &activeCredentialResponse{
				CredentialID: terminal.ActiveCredential.CredentialID,
				Version:      terminal.ActiveCredential.Version,
				ActivatedAt:  terminal.ActiveCredential.ActivatedAt,
			}
		}

		if terminal.Retiring != nil {
			response.Retiring = &retiringCredentialResponse{
				CredentialID: terminal.Retiring.CredentialID,
				Version:      terminal.Retiring.Version,
				RetireAt:     terminal.Retiring.RetireAt,
			}
		}

		items = append(items, response)
	}

	return items
}

func toProvisionResponse(output merchantterminaluc.ProvisionOutput) provisionResponse {
	return provisionResponse{
		MerchantID:        output.MerchantID,
		TerminalID:        output.TerminalID,
		CredentialID:      output.CredentialID,
		CredentialVersion: output.CredentialVersion,
		APIKey:            output.APIKey,
		CreatedAt:         output.CreatedAt,
		ExpiresAt:         output.ExpiresAt,
		Replayed:          output.Replayed,
	}
}

func toRotateInput(c *gin.Context) (merchantterminaluc.RotateInput, bool) {
	merchantID, ok := authenticatedMerchantID(c)
	if !ok {
		return merchantterminaluc.RotateInput{}, false
	}

	key, ok := idempotencyKey(c)
	if !ok {
		return merchantterminaluc.RotateInput{}, false
	}

	var grace *time.Duration

	// The body is optional: an empty body selects the default grace period.
	if c.Request.ContentLength != 0 {
		var req rotateRequest
		if !serdser.Bind(c, &req, binding.JSON) {
			return merchantterminaluc.RotateInput{}, false
		}

		if req.GracePeriodSeconds != nil {
			seconds := min(*req.GracePeriodSeconds, maxGraceSeconds)
			duration := time.Duration(seconds) * time.Second
			grace = &duration
		}
	}

	return merchantterminaluc.RotateInput{
		MerchantID:     merchantID,
		GracePeriod:    grace,
		IdempotencyKey: key,
	}, true
}

func toRotateResponse(output merchantterminaluc.RotateOutput) rotateResponse {
	response := rotateResponse{
		MerchantID:        output.MerchantID,
		TerminalID:        output.TerminalID,
		CredentialID:      output.CredentialID,
		CredentialVersion: output.CredentialVersion,
		APIKey:            output.APIKey,
		ActivatedAt:       output.ActivatedAt,
		ExpiresAt:         output.ExpiresAt,
		Replayed:          output.Replayed,
	}

	if output.Retiring != nil {
		response.Retiring = &retiringCredentialResponse{
			CredentialID: output.Retiring.CredentialID,
			Version:      output.Retiring.Version,
			RetireAt:     output.Retiring.RetireAt,
		}
	}

	return response
}

func toRetireInput(c *gin.Context) (merchantterminaluc.RetireInput, bool) {
	merchantID, ok := authenticatedMerchantID(c)
	if !ok {
		return merchantterminaluc.RetireInput{}, false
	}

	credentialID, err := model.ParseMerchantTerminalCredentialID(c.Param("credential_id"))
	if err != nil || credentialID.IsZero() {
		serdser.SerValidationErr(c, "credential_id", "validation.credential_id_invalid")

		return merchantterminaluc.RetireInput{}, false
	}

	return merchantterminaluc.RetireInput{
		MerchantID:   merchantID,
		CredentialID: credentialID,
	}, true
}

func toRetireResponse(output merchantterminaluc.RetireOutput) retireResponse {
	return retireResponse{
		CredentialID: output.CredentialID,
		Version:      output.Version,
		RetiredAt:    output.RetiredAt,
	}
}
