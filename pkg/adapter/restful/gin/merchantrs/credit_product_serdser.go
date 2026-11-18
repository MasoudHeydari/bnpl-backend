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

type setCreditProductRequest struct {
	// Enabled is the desired eligibility. It is required so an omitted body can
	// never silently disable a product.
	Enabled *bool `json:"enabled" binding:"required" example:"true"`
}

type creditProductResponse struct {
	CreditKind model.CreditKind `json:"credit_kind" example:"pay_next_month" enums:"pay_in_4,pay_next_month"`
	Enabled    bool             `json:"enabled" example:"true"`
}

type listCreditProductsResponse struct {
	MerchantProfileID model.MerchantProfileID `json:"merchant_profile_id" example:"8e1a2b3c-4d5e-4f6a-8b7c-9d0e1f2a3b4c" format:"uuid"`
	Products          []creditProductResponse `json:"products"`
}

type setCreditProductResponse struct {
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

func toListCreditProductsInput(c *gin.Context) (merchantuc.ListCreditProductsInput, bool) {
	merchantProfileID, ok := merchantProfileIDParam(c)
	if !ok {
		return merchantuc.ListCreditProductsInput{}, false
	}

	return merchantuc.ListCreditProductsInput{MerchantProfileID: merchantProfileID}, true
}

func toSetCreditProductInput(c *gin.Context) (merchantuc.SetCreditProductEnabledInput, bool) {
	actor, ok := adminSubject(c)
	if !ok {
		return merchantuc.SetCreditProductEnabledInput{}, false
	}

	merchantProfileID, ok := merchantProfileIDParam(c)
	if !ok {
		return merchantuc.SetCreditProductEnabledInput{}, false
	}

	kind := model.CreditKind(c.Param("credit_kind"))
	if !kind.IsValid() {
		serdser.SerValidationErr(c, "credit_kind", "validation.credit_kind_invalid")

		return merchantuc.SetCreditProductEnabledInput{}, false
	}

	var req setCreditProductRequest
	if !serdser.Bind(c, &req, binding.JSON) {
		return merchantuc.SetCreditProductEnabledInput{}, false
	}

	return merchantuc.SetCreditProductEnabledInput{
		ActorSubjectID:    actor,
		MerchantProfileID: merchantProfileID,
		CreditKind:        kind,
		Enabled:           *req.Enabled,
	}, true
}

func toListCreditProductsResponse(output merchantuc.ListCreditProductsOutput) listCreditProductsResponse {
	products := make([]creditProductResponse, 0, len(output.Products))
	for _, product := range output.Products {
		products = append(products, creditProductResponse{
			CreditKind: product.CreditKind,
			Enabled:    product.Enabled,
		})
	}

	return listCreditProductsResponse{
		MerchantProfileID: output.MerchantProfileID,
		Products:          products,
	}
}

func toSetCreditProductResponse(output merchantuc.SetCreditProductEnabledOutput) setCreditProductResponse {
	return setCreditProductResponse{
		MerchantProfileID: output.MerchantProfileID,
		CreditKind:        output.CreditKind,
		Enabled:           output.Enabled,
		Changed:           output.Changed,
	}
}
