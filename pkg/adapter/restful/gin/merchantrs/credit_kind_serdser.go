package merchantrs

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/permission"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/merchantuc"
)

type setCreditKindRequest struct {
	// Enabled is the desired eligibility. It is required so an omitted body can
	// never silently disable a credit kind.
	Enabled *bool `json:"enabled" binding:"required" example:"true"`
}

type creditKindResponse struct {
	CreditKind model.CreditKind `json:"credit_kind" example:"pay_next_month" enums:"pay_in_4,pay_next_month"`
	Enabled    bool             `json:"enabled" example:"true"`
}

type listCreditKindsResponse struct {
	MerchantProfileID model.MerchantProfileID `json:"merchant_profile_id" example:"8e1a2b3c-4d5e-4f6a-8b7c-9d0e1f2a3b4c" format:"uuid"`
	CreditKinds       []creditKindResponse    `json:"credit_kinds"`
}

type setCreditKindResponse struct {
	MerchantProfileID model.MerchantProfileID `json:"merchant_profile_id" example:"8e1a2b3c-4d5e-4f6a-8b7c-9d0e1f2a3b4c" format:"uuid"`
	CreditKind        model.CreditKind        `json:"credit_kind" example:"pay_next_month" enums:"pay_in_4,pay_next_month"`
	Enabled           bool                    `json:"enabled" example:"true"`
	Changed           bool                    `json:"changed" example:"true"`
}

func adminSubject(c *gin.Context) (model.SubjectID, bool) {
	const op = "merchantrs.adminSubject"

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
						"x-auth-subject-id",
						"x-auth-tenant-id",
						"x-auth-context-id",
					},
					Violations: []cerr.Violation{
						{
							Code:       "REQUIRED",
							MessageKey: "error.trusted_authentication_headers_are_required",
							Fields: []string{
								"x-auth-subject-id",
								"x-auth-tenant-id",
								"x-auth-context-id",
							},
						},
					},
				},
			),
		)

		return model.SubjectID{}, false
	}

	return authCtx.SubjectID, true
}

func merchantProfileIDParam(c *gin.Context) (model.MerchantProfileID, bool) {
	id, err := model.ParseMerchantProfileID(c.Param("merchant_profile_id"))
	if err != nil || id.IsZero() {
		serdser.SerValidationErr(c, "merchant_profile_id", "validation.merchant_profile_id_invalid")

		return model.MerchantProfileID{}, false
	}

	return id, true
}

func toListCreditKindsInput(c *gin.Context) (merchantuc.ListCreditKindsInput, bool) {
	merchantProfileID, ok := merchantProfileIDParam(c)
	if !ok {
		return merchantuc.ListCreditKindsInput{}, false
	}

	return merchantuc.ListCreditKindsInput{MerchantProfileID: merchantProfileID}, true
}

func toSetCreditKindInput(c *gin.Context) (merchantuc.SetCreditKindEnabledInput, bool) {
	actor, ok := adminSubject(c)
	if !ok {
		return merchantuc.SetCreditKindEnabledInput{}, false
	}

	merchantProfileID, ok := merchantProfileIDParam(c)
	if !ok {
		return merchantuc.SetCreditKindEnabledInput{}, false
	}

	kind := model.CreditKind(c.Param("credit_kind"))
	if !kind.IsValid() {
		serdser.SerValidationErr(c, "credit_kind", "validation.credit_kind_invalid")

		return merchantuc.SetCreditKindEnabledInput{}, false
	}

	var req setCreditKindRequest
	if !serdser.Bind(c, &req, binding.JSON) {
		return merchantuc.SetCreditKindEnabledInput{}, false
	}

	return merchantuc.SetCreditKindEnabledInput{
		ActorSubjectID:    actor,
		MerchantProfileID: merchantProfileID,
		CreditKind:        kind,
		Enabled:           *req.Enabled,
	}, true
}

func toListCreditKindsResponse(output merchantuc.ListCreditKindsOutput) listCreditKindsResponse {
	kinds := make([]creditKindResponse, 0, len(output.CreditKinds))
	for _, kind := range output.CreditKinds {
		kinds = append(kinds, creditKindResponse{
			CreditKind: kind.CreditKind,
			Enabled:    kind.Enabled,
		})
	}

	return listCreditKindsResponse{
		MerchantProfileID: output.MerchantProfileID,
		CreditKinds:       kinds,
	}
}

func toSetCreditKindResponse(output merchantuc.SetCreditKindEnabledOutput) setCreditKindResponse {
	return setCreditKindResponse{
		MerchantProfileID: output.MerchantProfileID,
		CreditKind:        output.CreditKind,
		Enabled:           output.Enabled,
		Changed:           output.Changed,
	}
}
