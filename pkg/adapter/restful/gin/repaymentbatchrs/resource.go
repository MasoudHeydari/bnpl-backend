// Package repaymentbatchrs exposes authenticated aggregate repayment queries
// and start commands. Plan assessment and accounting references are never
// accepted from the customer request body.
package repaymentbatchrs

import (
	"fmt"
	"net/http"
	"github.com/gin-gonic/gin/binding"
	"github.com/parstis/bnpl-backend/pkg/core/model"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/middleware"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/header"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	extcollection "github.com/parstis/bnpl-backend/pkg/core/external/collectionpayment"
	"github.com/parstis/bnpl-backend/pkg/core/permission"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/repaymentbatchuc"
)

type RegisterOptions struct{
	ProviderCode string
	ReturnURL string
}
type resource struct{
	uc *repaymentbatchuc.UseCase
	providerCode string
	urls extcollection.ReturnURLs
}
func Register(router *gin.RouterGroup,uc *repaymentbatchuc.UseCase,opts RegisterOptions){
	if uc==nil{return}
	auth:=middleware.AuthContext()
	rs:=&resource{uc:uc,providerCode:opts.ProviderCode,urls:extcollection.ReturnURLs{
		Success:opts.ReturnURL,Failure:opts.ReturnURL,Cancel:opts.ReturnURL,
	}}
	router.POST("/repayment-batches",auth.RequireAny(permission.PaymentsStartSelf),rs.Create)
	router.GET("/repayment-batches/:id",auth.RequireAny(permission.PaymentsReadSelf),rs.Get)
	router.POST("/repayment-batches/:id/start",auth.RequireAny(permission.PaymentsStartSelf),rs.Start)
}

type statusResponse struct{
	ID uuid.UUID `json:"id"`
	Status string `json:"status"`
	TotalAmountMinor uint64 `json:"total_amount_minor"`
	Currency string `json:"currency"`
}
type startResponse struct{
	ID uuid.UUID `json:"id"`
	Status string `json:"status"`
	RedirectURL string `json:"redirect_url,omitempty"`
}

func input(c *gin.Context)(uuid.UUID,permission.AuthContext,bool){
	auth,ok:=permission.FromContext(c.Request.Context())
	if !ok{serdser.SerErr(c,cerr.Unauthorized(fmt.Errorf("authenticated customer is required")));return uuid.Nil,permission.AuthContext{},false}
	id,err:=uuid.Parse(c.Param("id"))
	if err!=nil||id==uuid.Nil{
		serdser.SerValidationErr(c,"id","validation.repayment_batch_id_invalid")
		return uuid.Nil,permission.AuthContext{},false
	}
	return id,auth,true
}

// Get reports BNPL's durable local status without exposing FC internal IDs.
// A background reconciliation task independently refreshes this projection.
func(rs *resource)Get(c *gin.Context){
	id,auth,ok:=input(c);if !ok{return}
	row,err:=rs.uc.RequireCustomerBatch(c.Request.Context(),auth.SubjectID.AsExternalSubjectID(),id)
	if err!=nil{serdser.SerErr(c,err);return}
	serdser.Ser(c,http.StatusOK,"REPAYMENT_BATCH_RETRIEVED",
		"success.repayment_batch_retrieved",
		statusResponse{ID:row.ID,Status:string(row.Status),TotalAmountMinor:row.TotalAmountMinor,Currency:string(row.Currency)})
}

// Start uses server-configured payment destination/provider, never the
// browser's return URL or an untrusted merchant-provided provider code.
func(rs *resource)Start(c *gin.Context){
	id,auth,ok:=input(c);if !ok{return}
	key:=c.GetHeader(header.IdempotencyKey)
	if key==""||len(key)>256{
		serdser.SerValidationErr(c,header.IdempotencyKey,"validation.idempotency_key_header_required")
		return
	}
	if rs.providerCode==""||!rs.urls.IsComplete(){
		serdser.SerErr(c,cerr.InternalError(fmt.Errorf("repayment provider/return URL are not configured")))
		return
	}
	result,err:=rs.uc.StartForCustomer(c.Request.Context(),auth.SubjectID.AsExternalSubjectID(),
		id,key,rs.providerCode,rs.urls)
	if err!=nil{serdser.SerErr(c,err);return}
	serdser.Ser(c,http.StatusAccepted,"REPAYMENT_BATCH_STARTED",
		"success.repayment_batch_started",
		startResponse{ID:id,Status:string(result.Status),RedirectURL:result.PaymentLink})
}


// Create takes only BNPL installment IDs. The server loads authoritative
// customer obligations, validates their payable amounts and resolves FC
// economic references using a trusted service, never customer-supplied rules.
type createRequest struct {
	InstallmentIDs []string `json:"installment_ids" binding:"required,min=1,max=50"`
}

// Create handles POST /api/v1/repayment-batches. A missing FC provisioning
// integration returns a service-unavailable error and creates no payment.
func (rs *resource) Create(c *gin.Context) {
	auth, ok := permission.FromContext(c.Request.Context())
	if !ok {
		serdser.SerErr(c, cerr.Unauthorized(fmt.Errorf("authenticated customer required")))
		return
	}
	key := c.GetHeader(header.IdempotencyKey)
	if key == "" || len(key) > 256 {
		serdser.SerValidationErr(c, header.IdempotencyKey, "validation.idempotency_key_header_required")
		return
	}
	var req createRequest
	if !serdser.Bind(c, &req, binding.JSON) {
		return
	}
	ids := make([]model.InstallmentID, 0, len(req.InstallmentIDs))
	for _, raw := range req.InstallmentIDs {
		id, err := model.ParseInstallmentID(raw)
		if err != nil || id.IsZero() {
			serdser.SerValidationErr(c, "installment_ids", "validation.installment_id_invalid")
			return
		}
		ids = append(ids, id)
	}
	row, err := rs.uc.CreateForCustomer(
		c.Request.Context(), auth.SubjectID.AsExternalSubjectID(), ids, key,
	)
	if err != nil {
		serdser.SerErr(c, err)
		return
	}
	serdser.Ser(c,http.StatusAccepted,"REPAYMENT_BATCH_CREATED",
		"success.repayment_batch_created",
		statusResponse{ID:row.ID,Status:string(row.Status),
			TotalAmountMinor:row.TotalAmountMinor,Currency:string(row.Currency)})
}
