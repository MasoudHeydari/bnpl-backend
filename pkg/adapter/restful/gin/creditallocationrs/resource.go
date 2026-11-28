package creditallocationrs

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/middleware"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/permission"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/creditallocationuc"
)

type resource struct {
	allocation         *creditallocationuc.UseCase
	importMaxItems     int
	importMaxBodyBytes int64
}

// RegisterOptions defines transport limits for administrative imports.
type RegisterOptions struct {
	ImportMaxItems     int
	ImportMaxBodyBytes int64
}

// Register attaches admin-only routes; all actor IDs are obtained from trusted
// authentication context, not from client-provided JSON.
func Register(router *gin.RouterGroup, uc *creditallocationuc.UseCase, opts RegisterOptions) {
	if uc == nil {
		return
	}
	rs := &resource{allocation: uc, importMaxItems: opts.ImportMaxItems, importMaxBodyBytes: opts.ImportMaxBodyBytes}
	auth := middleware.AuthContext()
	read := auth.RequireAny(permission.CreditAllocationsReadAny)
	manage := auth.RequireAny(permission.CreditAllocationsManageAny)
	router.GET("/admin/credits/groups", read, rs.ListGroups)
	router.PUT("/admin/credits/groups/:group_code", manage, rs.PutGroup)
	router.GET("/admin/credits/eligibility", read, rs.ListEligibility)
	router.GET("/admin/credits/eligibility/:eligibility_id", read, rs.GetEligibility)
	router.PUT("/admin/credits/eligibility", manage, rs.PutEligibility)
	router.POST("/admin/credits/eligibility/import", manage, rs.ImportEligibility)
	router.GET("/admin/credits/allocation/rules", read, rs.ListRules)
	router.PUT("/admin/credits/allocation/rules", manage, rs.PutRule)
	router.GET("/admin/credits/allocation/defaults", read, rs.ListDefaults)
	router.PUT("/admin/credits/allocation/defaults", manage, rs.PutDefault)
	router.GET("/admin/credits/requests", auth.RequireAny(permission.CreditRequestsReadAny), rs.ListRequests)
}

func actor(c *gin.Context) (string, bool) {
	auth, ok := permission.FromContext(c.Request.Context())
	if !ok {
		serdser.SerErr(c, cerr.Unauthorized(fmt.Errorf("authentication context missing")))
		return "", false
	}
	return auth.SubjectID.String(), true
}
func write(c *gin.Context, data any, err error) {
	if err != nil {
		serdser.SerErr(c, err)
		return
	}
	serdser.Ser(c, http.StatusOK, serdser.SuccessCodeCreditAllocationConfigured, serdser.SuccessMessageCreditAllocationConfigured, data)
}
func read(c *gin.Context, data any, err error) {
	if err != nil {
		serdser.SerErr(c, err)
		return
	}
	serdser.Ser(c, http.StatusOK, serdser.SuccessCodeCreditAllocationRetrieved, serdser.SuccessMessageCreditAllocationRetrieved, data)
}
func bad(c *gin.Context, field string) {
	serdser.SerErr(c, cerr.InvalidArgument(fmt.Errorf("invalid %s", field)))
}
func limit(c *gin.Context) (int, bool) {
	n, err := strconv.Atoi(c.DefaultQuery("limit", "100"))
	if err != nil || n < 1 || n > 500 {
		bad(c, "limit")
		return 0, false
	}
	return n, true
}
func idParam(c *gin.Context, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil || id == uuid.Nil {
		bad(c, name)
		return uuid.Nil, false
	}
	return id, true
}

type groupRequest struct {
	Name    string `json:"name" binding:"required"`
	Enabled *bool  `json:"enabled" binding:"required"`
}

// ListGroups handles GET /api/v1/admin/credits/groups.
//
// @Summary List credit groups
// @Description Requires bnpl:credit:allocations:read:any.
// @Tags Credit Allocation Admin
// @Produce json
// @Param x-auth-subject-id header string true "Authenticated administrator subject UUID issued by Identity Service" format(uuid)
// @Param x-auth-tenant-id header string true "Authenticated tenant UUID" format(uuid)
// @Param x-auth-context-id header string true "Authenticated business-context UUID" format(uuid)
// @Param x-auth-permissions header string true "Comma-separated gateway permissions"
// @Param X-Trace-Id header string false "Optional; reused when present otherwise generated. Always returned."
// @Param X-Request-Id header string false "Optional; reused when present otherwise generated. Always returned."
// @Param X-Correlation-Id header string false "Optional; reused when present otherwise generated. Always returned."
// @Success 200 {object} ListGroupsSuccessSchema
// @Failure 400 {object} serdser.ErrorResponse
// @Failure 401 {object} serdser.ErrorResponse
// @Failure 403 {object} serdser.ErrorResponse
// @Failure 404 {object} serdser.ErrorResponse
// @Failure 409 {object} serdser.ErrorResponse
// @Failure 422 {object} serdser.ErrorResponse
// @Failure 500 {object} serdser.ErrorResponse
// @Security PermissionAuth
// @Router /api/v1/admin/credits/groups [get]
func (rs *resource) ListGroups(c *gin.Context) {
	out, err := rs.allocation.ListGroups(c.Request.Context())
	read(c, out, err)
}

// PutGroup handles PUT /api/v1/admin/credits/groups/{group_code}.
//
// @Summary Create or update group; existing credits are unchanged
// @Description Requires bnpl:credit:allocations:manage:any.
// @Tags Credit Allocation Admin
// @Produce json
// @Accept json
// @Param group_code path string true "group_code"
// @Param x-auth-subject-id header string true "Authenticated administrator subject UUID issued by Identity Service" format(uuid)
// @Param x-auth-tenant-id header string true "Authenticated tenant UUID" format(uuid)
// @Param x-auth-context-id header string true "Authenticated business-context UUID" format(uuid)
// @Param x-auth-permissions header string true "Comma-separated gateway permissions"
// @Param X-Trace-Id header string false "Optional; reused when present otherwise generated. Always returned."
// @Param X-Request-Id header string false "Optional; reused when present otherwise generated. Always returned."
// @Param X-Correlation-Id header string false "Optional; reused when present otherwise generated. Always returned."
// @Param body body groupRequest true "Request body"
// @Success 200 {object} PutGroupSuccessSchema
// @Failure 400 {object} serdser.ErrorResponse
// @Failure 401 {object} serdser.ErrorResponse
// @Failure 403 {object} serdser.ErrorResponse
// @Failure 404 {object} serdser.ErrorResponse
// @Failure 409 {object} serdser.ErrorResponse
// @Failure 422 {object} serdser.ErrorResponse
// @Failure 500 {object} serdser.ErrorResponse
// @Security PermissionAuth
// @Router /api/v1/admin/credits/groups/{group_code} [put]
func (rs *resource) PutGroup(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	var req groupRequest
	if !serdser.Bind(c, &req, binding.JSON) {
		return
	}
	out, err := rs.allocation.PutGroup(c.Request.Context(), c.Param("group_code"), req.Name, *req.Enabled, a)
	write(c, out, err)
}

// ListEligibility handles GET /api/v1/admin/credits/eligibility.
//
// @Summary List eligibility entries; cursor is the last entry ID
// @Description Requires bnpl:credit:allocations:read:any.
// @Tags Credit Allocation Admin
// @Produce json
// @Param group_code query string false "group_code"
// @Param after query string false "after" format(uuid)
// @Param limit query integer false "limit"
// @Param x-auth-subject-id header string true "Authenticated administrator subject UUID issued by Identity Service" format(uuid)
// @Param x-auth-tenant-id header string true "Authenticated tenant UUID" format(uuid)
// @Param x-auth-context-id header string true "Authenticated business-context UUID" format(uuid)
// @Param x-auth-permissions header string true "Comma-separated gateway permissions"
// @Param X-Trace-Id header string false "Optional; reused when present otherwise generated. Always returned."
// @Param X-Request-Id header string false "Optional; reused when present otherwise generated. Always returned."
// @Param X-Correlation-Id header string false "Optional; reused when present otherwise generated. Always returned."
// @Success 200 {object} ListEligibilitySuccessSchema
// @Failure 400 {object} serdser.ErrorResponse
// @Failure 401 {object} serdser.ErrorResponse
// @Failure 403 {object} serdser.ErrorResponse
// @Failure 404 {object} serdser.ErrorResponse
// @Failure 409 {object} serdser.ErrorResponse
// @Failure 422 {object} serdser.ErrorResponse
// @Failure 500 {object} serdser.ErrorResponse
// @Security PermissionAuth
// @Router /api/v1/admin/credits/eligibility [get]
func (rs *resource) ListEligibility(c *gin.Context) {
	n, ok := limit(c)
	if !ok {
		return
	}
	var group *string
	if v := c.Query("group_code"); v != "" {
		group = &v
	}
	var after *uuid.UUID
	if v := c.Query("after"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			bad(c, "after")
			return
		}
		after = &id
	}
	out, err := rs.allocation.ListEligibility(c.Request.Context(), group, after, n)
	read(c, out, err)
}

// GetEligibility handles GET /api/v1/admin/credits/eligibility/{eligibility_id}.
//
// @Summary Read eligibility, mobile aliases and current membership
// @Description Requires bnpl:credit:allocations:read:any.
// @Tags Credit Allocation Admin
// @Produce json
// @Param eligibility_id path string true "eligibility_id" format(uuid)
// @Param x-auth-subject-id header string true "Authenticated administrator subject UUID issued by Identity Service" format(uuid)
// @Param x-auth-tenant-id header string true "Authenticated tenant UUID" format(uuid)
// @Param x-auth-context-id header string true "Authenticated business-context UUID" format(uuid)
// @Param x-auth-permissions header string true "Comma-separated gateway permissions"
// @Param X-Trace-Id header string false "Optional; reused when present otherwise generated. Always returned."
// @Param X-Request-Id header string false "Optional; reused when present otherwise generated. Always returned."
// @Param X-Correlation-Id header string false "Optional; reused when present otherwise generated. Always returned."
// @Success 200 {object} GetEligibilitySuccessSchema
// @Failure 400 {object} serdser.ErrorResponse
// @Failure 401 {object} serdser.ErrorResponse
// @Failure 403 {object} serdser.ErrorResponse
// @Failure 404 {object} serdser.ErrorResponse
// @Failure 409 {object} serdser.ErrorResponse
// @Failure 422 {object} serdser.ErrorResponse
// @Failure 500 {object} serdser.ErrorResponse
// @Security PermissionAuth
// @Router /api/v1/admin/credits/eligibility/{eligibility_id} [get]
func (rs *resource) GetEligibility(c *gin.Context) {
	id, ok := idParam(c, "eligibility_id")
	if !ok {
		return
	}
	out, err := rs.allocation.GetEligibility(c.Request.Context(), id)
	read(c, out, err)
}

// PutEligibility handles PUT /api/v1/admin/credits/eligibility.
//
// @Summary Register eligibility or replace group membership; null removes membership
// @Description Requires bnpl:credit:allocations:manage:any.
// @Tags Credit Allocation Admin
// @Produce json
// @Accept json
// @Param x-auth-subject-id header string true "Authenticated administrator subject UUID issued by Identity Service" format(uuid)
// @Param x-auth-tenant-id header string true "Authenticated tenant UUID" format(uuid)
// @Param x-auth-context-id header string true "Authenticated business-context UUID" format(uuid)
// @Param x-auth-permissions header string true "Comma-separated gateway permissions"
// @Param X-Trace-Id header string false "Optional; reused when present otherwise generated. Always returned."
// @Param X-Request-Id header string false "Optional; reused when present otherwise generated. Always returned."
// @Param X-Correlation-Id header string false "Optional; reused when present otherwise generated. Always returned."
// @Param body body creditallocationuc.EligibilityInput true "Request body"
// @Success 200 {object} PutEligibilitySuccessSchema
// @Failure 400 {object} serdser.ErrorResponse
// @Failure 401 {object} serdser.ErrorResponse
// @Failure 403 {object} serdser.ErrorResponse
// @Failure 404 {object} serdser.ErrorResponse
// @Failure 409 {object} serdser.ErrorResponse
// @Failure 422 {object} serdser.ErrorResponse
// @Failure 500 {object} serdser.ErrorResponse
// @Security PermissionAuth
// @Router /api/v1/admin/credits/eligibility [put]
func (rs *resource) PutEligibility(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	var req creditallocationuc.EligibilityInput
	if !serdser.Bind(c, &req, binding.JSON) {
		return
	}
	out, err := rs.allocation.ImportEligibility(c.Request.Context(), []creditallocationuc.EligibilityInput{req}, a)
	write(c, out, err)
}

// parseEligibilityJSON accepts exactly one JSON array and limits decoded rows.
// A malformed body never becomes a partially applied import.
func parseEligibilityJSON(body io.Reader, maxItems int) ([]creditallocationuc.EligibilityInput, error) {
	d := json.NewDecoder(body)
	d.DisallowUnknownFields()
	token, err := d.Token()
	if err != nil {
		return nil, err
	}
	if token != json.Delim('[') {
		return nil, fmt.Errorf("expected eligibility array")
	}
	rows := make([]creditallocationuc.EligibilityInput, 0)
	for d.More() {
		if len(rows) >= maxItems {
			return nil, fmt.Errorf("eligibility item limit exceeded")
		}
		var row creditallocationuc.EligibilityInput
		if err := d.Decode(&row); err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	if _, err := d.Token(); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("empty eligibility array")
	}
	if err := d.Decode(new(json.RawMessage)); err != io.EOF {
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("unexpected trailing JSON")
	}
	return rows, nil
}

// requireJSON refuses alternate content types before parsing a request body.
func requireJSON(c *gin.Context) bool {
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || mediaType != "application/json" {
		serdser.SerErr(c, cerr.NewPublic(cerr.CodeUnsupportedMediaType, cerr.DefaultMessageKey(cerr.CodeUnsupportedMediaType), nil, cerr.Details{}))
		return false
	}
	return true
}

func jsonBodyError(c *gin.Context, err error) {
	var maxBytes *http.MaxBytesError
	if errors.As(err, &maxBytes) {
		serdser.SerErr(c, cerr.NewPublic(cerr.CodePayloadTooLarge, cerr.DefaultMessageKey(cerr.CodePayloadTooLarge), nil, cerr.Details{}))
		return
	}
	// Do not log decoder errors that could include imported identity data.
	bad(c, "body")
}

// bindRuleJSON uses a strict single object so defaults cannot accept an amount
// or other configuration fields that have no meaning for public assessment.
func bindRuleJSON(c *gin.Context, req any) bool {
	if !requireJSON(c) {
		return false
	}
	d := json.NewDecoder(c.Request.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(req); err != nil {
		jsonBodyError(c, err)
		return false
	}
	if err := d.Decode(new(json.RawMessage)); err != io.EOF {
		jsonBodyError(c, err)
		return false
	}
	return true
}

// ImportEligibility handles POST /api/v1/admin/credits/eligibility/import.
//
// @Summary Atomically import a JSON array (default 1000 items and 2 MiB); null group removes membership
// @Description Requires bnpl:credit:allocations:manage:any.
// @Tags Credit Allocation Admin
// @Produce json
// @Accept json
// @Param x-auth-subject-id header string true "Authenticated administrator subject UUID issued by Identity Service" format(uuid)
// @Param x-auth-tenant-id header string true "Authenticated tenant UUID" format(uuid)
// @Param x-auth-context-id header string true "Authenticated business-context UUID" format(uuid)
// @Param x-auth-permissions header string true "Comma-separated gateway permissions"
// @Param X-Trace-Id header string false "Optional; reused when present otherwise generated. Always returned."
// @Param X-Request-Id header string false "Optional; reused when present otherwise generated. Always returned."
// @Param X-Correlation-Id header string false "Optional; reused when present otherwise generated. Always returned."
// @Param body body []creditallocationuc.EligibilityInput true "Eligibility entries (maximum configured by environment)"
// @Success 200 {object} ImportEligibilitySuccessSchema
// @Header 200 {string} X-Trace-Id "Always set: reuse inbound value or generate"
// @Header 200 {string} X-Request-Id "Always set: reuse inbound value or generate"
// @Header 200 {string} X-Correlation-Id "Always set: reuse inbound value or generate"
// @Failure 413 {object} serdser.ErrorResponse
// @Header 413 {string} X-Trace-Id "Always set: reuse inbound value or generate"
// @Header 413 {string} X-Request-Id "Always set: reuse inbound value or generate"
// @Header 413 {string} X-Correlation-Id "Always set: reuse inbound value or generate"
// @Failure 415 {object} serdser.ErrorResponse
// @Header 415 {string} X-Trace-Id "Always set: reuse inbound value or generate"
// @Header 415 {string} X-Request-Id "Always set: reuse inbound value or generate"
// @Header 415 {string} X-Correlation-Id "Always set: reuse inbound value or generate"
// @Failure 400 {object} serdser.ErrorResponse
// @Failure 401 {object} serdser.ErrorResponse
// @Failure 403 {object} serdser.ErrorResponse
// @Failure 404 {object} serdser.ErrorResponse
// @Failure 409 {object} serdser.ErrorResponse
// @Failure 422 {object} serdser.ErrorResponse
// @Failure 500 {object} serdser.ErrorResponse
// @Security PermissionAuth
// @Router /api/v1/admin/credits/eligibility/import [post]
func (rs *resource) ImportEligibility(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	if !requireJSON(c) {
		return
	}
	if c.Request.ContentLength > rs.importMaxBodyBytes {
		jsonBodyError(c, &http.MaxBytesError{Limit: rs.importMaxBodyBytes})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, rs.importMaxBodyBytes)
	rows, err := parseEligibilityJSON(c.Request.Body, rs.importMaxItems)
	if err != nil {
		jsonBodyError(c, err)
		return
	}
	out, err := rs.allocation.ImportEligibility(c.Request.Context(), rows, a)
	write(c, out, err)
}

// ListRules handles GET /api/v1/admin/credits/allocation/rules.
//
// @Summary List current group and individual allocation rules
// @Description Requires bnpl:credit:allocations:read:any.
// @Tags Credit Allocation Admin
// @Produce json
// @Param x-auth-subject-id header string true "Authenticated administrator subject UUID issued by Identity Service" format(uuid)
// @Param x-auth-tenant-id header string true "Authenticated tenant UUID" format(uuid)
// @Param x-auth-context-id header string true "Authenticated business-context UUID" format(uuid)
// @Param x-auth-permissions header string true "Comma-separated gateway permissions"
// @Param X-Trace-Id header string false "Optional; reused when present otherwise generated. Always returned."
// @Param X-Request-Id header string false "Optional; reused when present otherwise generated. Always returned."
// @Param X-Correlation-Id header string false "Optional; reused when present otherwise generated. Always returned."
// @Success 200 {object} ListRulesSuccessSchema
// @Failure 400 {object} serdser.ErrorResponse
// @Failure 401 {object} serdser.ErrorResponse
// @Failure 403 {object} serdser.ErrorResponse
// @Failure 404 {object} serdser.ErrorResponse
// @Failure 409 {object} serdser.ErrorResponse
// @Failure 422 {object} serdser.ErrorResponse
// @Failure 500 {object} serdser.ErrorResponse
// @Security PermissionAuth
// @Router /api/v1/admin/credits/allocation/rules [get]
func (rs *resource) ListRules(c *gin.Context) {
	out, err := rs.allocation.ListRules(c.Request.Context())
	read(c, out, err)
}

// PutRule handles PUT /api/v1/admin/credits/allocation/rules.
//
// @Summary Append a group or individual allocation rule revision. Default amounts are forbidden.
// @Description Requires bnpl:credit:allocations:manage:any.
// @Tags Credit Allocation Admin
// @Produce json
// @Accept json
// @Param x-auth-subject-id header string true "Authenticated administrator subject UUID issued by Identity Service" format(uuid)
// @Param x-auth-tenant-id header string true "Authenticated tenant UUID" format(uuid)
// @Param x-auth-context-id header string true "Authenticated business-context UUID" format(uuid)
// @Param x-auth-permissions header string true "Comma-separated gateway permissions"
// @Param X-Trace-Id header string false "Optional; reused when present otherwise generated. Always returned."
// @Param X-Request-Id header string false "Optional; reused when present otherwise generated. Always returned."
// @Param X-Correlation-Id header string false "Optional; reused when present otherwise generated. Always returned."
// @Param body body creditallocationuc.RuleInput true "Request body"
// @Success 200 {object} PutRuleSuccessSchema
// @Header 200 {string} X-Trace-Id "Always set: reuse inbound value or generate"
// @Header 200 {string} X-Request-Id "Always set: reuse inbound value or generate"
// @Header 200 {string} X-Correlation-Id "Always set: reuse inbound value or generate"
// @Failure 415 {object} serdser.ErrorResponse
// @Header 415 {string} X-Trace-Id "Always set: reuse inbound value or generate"
// @Header 415 {string} X-Request-Id "Always set: reuse inbound value or generate"
// @Header 415 {string} X-Correlation-Id "Always set: reuse inbound value or generate"
// @Failure 400 {object} serdser.ErrorResponse
// @Failure 401 {object} serdser.ErrorResponse
// @Failure 403 {object} serdser.ErrorResponse
// @Failure 404 {object} serdser.ErrorResponse
// @Failure 409 {object} serdser.ErrorResponse
// @Failure 422 {object} serdser.ErrorResponse
// @Failure 500 {object} serdser.ErrorResponse
// @Security PermissionAuth
// @Router /api/v1/admin/credits/allocation/rules [put]
func (rs *resource) PutRule(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	var req creditallocationuc.RuleInput
	if !bindRuleJSON(c, &req) {
		return
	}
	out, err := rs.allocation.PutRule(c.Request.Context(), req, a)
	write(c, out, err)
}

// ListRequests handles GET /api/v1/admin/credits/requests.
//
// @Summary List pending or processing requests for a product
// @Description Requires bnpl:credit:requests:read:any.
// @Tags Credit Allocation Admin
// @Produce json
// @Param credit_kind query string true "credit_kind"
// @Param before query string false "before" format(date-time)
// @Param limit query integer false "limit"
// @Param x-auth-subject-id header string true "Authenticated administrator subject UUID issued by Identity Service" format(uuid)
// @Param x-auth-tenant-id header string true "Authenticated tenant UUID" format(uuid)
// @Param x-auth-context-id header string true "Authenticated business-context UUID" format(uuid)
// @Param x-auth-permissions header string true "Comma-separated gateway permissions"
// @Param X-Trace-Id header string false "Optional; reused when present otherwise generated. Always returned."
// @Param X-Request-Id header string false "Optional; reused when present otherwise generated. Always returned."
// @Param X-Correlation-Id header string false "Optional; reused when present otherwise generated. Always returned."
// @Success 200 {object} ListRequestsSuccessSchema
// @Failure 400 {object} serdser.ErrorResponse
// @Failure 401 {object} serdser.ErrorResponse
// @Failure 403 {object} serdser.ErrorResponse
// @Failure 404 {object} serdser.ErrorResponse
// @Failure 409 {object} serdser.ErrorResponse
// @Failure 422 {object} serdser.ErrorResponse
// @Failure 500 {object} serdser.ErrorResponse
// @Security PermissionAuth
// @Router /api/v1/admin/credits/requests [get]
func (rs *resource) ListRequests(c *gin.Context) {
	n, ok := limit(c)
	if !ok {
		return
	}
	var before time.Time
	if v := c.Query("before"); v != "" {
		var err error
		before, err = time.Parse(time.RFC3339Nano, v)
		if err != nil {
			bad(c, "before")
			return
		}
	}
	out, err := rs.allocation.ListPendingRequests(c.Request.Context(), model.CreditKind(c.Query("credit_kind")), before, n)
	read(c, out, err)
}

// ListDefaults handles GET /api/v1/admin/credits/allocation/defaults.
//
// @Summary List current public allocation settings
// @Description Requires bnpl:credit:allocations:read:any.
// @Tags Credit Allocation Admin
// @Produce json
// @Param x-auth-subject-id header string true "Authenticated administrator subject UUID issued by Identity Service" format(uuid)
// @Param x-auth-tenant-id header string true "Authenticated tenant UUID" format(uuid)
// @Param x-auth-context-id header string true "Authenticated business-context UUID" format(uuid)
// @Param x-auth-permissions header string true "Comma-separated gateway permissions"
// @Param X-Trace-Id header string false "Optional; reused when present otherwise generated. Always returned."
// @Param X-Request-Id header string false "Optional; reused when present otherwise generated. Always returned."
// @Param X-Correlation-Id header string false "Optional; reused when present otherwise generated. Always returned."
// @Success 200 {object} ListRulesSuccessSchema
// @Failure 400 {object} serdser.ErrorResponse
// @Failure 401 {object} serdser.ErrorResponse
// @Failure 403 {object} serdser.ErrorResponse
// @Failure 404 {object} serdser.ErrorResponse
// @Failure 409 {object} serdser.ErrorResponse
// @Failure 422 {object} serdser.ErrorResponse
// @Failure 500 {object} serdser.ErrorResponse
// @Security PermissionAuth
// @Router /api/v1/admin/credits/allocation/defaults [get]
func (rs *resource) ListDefaults(c *gin.Context) {
	out, err := rs.allocation.ListDefaults(c.Request.Context())
	read(c, out, err)
}

// PutDefault handles PUT /api/v1/admin/credits/allocation/defaults.
//
// @Summary Enable or disable public risk assessment. Enabling is unavailable until a real assessor is implemented.
// @Description Requires bnpl:credit:allocations:manage:any.
// @Tags Credit Allocation Admin
// @Produce json
// @Accept json
// @Param x-auth-subject-id header string true "Authenticated administrator subject UUID issued by Identity Service" format(uuid)
// @Param x-auth-tenant-id header string true "Authenticated tenant UUID" format(uuid)
// @Param x-auth-context-id header string true "Authenticated business-context UUID" format(uuid)
// @Param x-auth-permissions header string true "Comma-separated gateway permissions"
// @Param X-Trace-Id header string false "Optional; reused when present otherwise generated. Always returned."
// @Param X-Request-Id header string false "Optional; reused when present otherwise generated. Always returned."
// @Param X-Correlation-Id header string false "Optional; reused when present otherwise generated. Always returned."
// @Param body body creditallocationuc.DefaultInput true "Request body"
// @Success 200 {object} PutRuleSuccessSchema
// @Header 200 {string} X-Trace-Id "Always set: reuse inbound value or generate"
// @Header 200 {string} X-Request-Id "Always set: reuse inbound value or generate"
// @Header 200 {string} X-Correlation-Id "Always set: reuse inbound value or generate"
// @Failure 415 {object} serdser.ErrorResponse
// @Header 415 {string} X-Trace-Id "Always set: reuse inbound value or generate"
// @Header 415 {string} X-Request-Id "Always set: reuse inbound value or generate"
// @Header 415 {string} X-Correlation-Id "Always set: reuse inbound value or generate"
// @Failure 400 {object} serdser.ErrorResponse
// @Failure 401 {object} serdser.ErrorResponse
// @Failure 403 {object} serdser.ErrorResponse
// @Failure 404 {object} serdser.ErrorResponse
// @Failure 409 {object} serdser.ErrorResponse
// @Failure 422 {object} serdser.ErrorResponse
// @Failure 500 {object} serdser.ErrorResponse
// @Security PermissionAuth
// @Router /api/v1/admin/credits/allocation/defaults [put]
func (rs *resource) PutDefault(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	var req creditallocationuc.DefaultInput
	if !bindRuleJSON(c, &req) {
		return
	}
	out, err := rs.allocation.PutDefault(c.Request.Context(), req, a)
	write(c, out, err)
}
