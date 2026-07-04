package serdser

import (
	"time"

	"github.com/gin-gonic/gin"

	corepagination "github.com/parstis/bnpl-backend/pkg/core/pagination"
	"github.com/parstis/bnpl-backend/pkg/core/trace"
)

// SuccessCode identifies a stable successful response category.
//
// Success codes are machine-readable identifiers intended for clients,
// frontend applications, monitoring, and API analytics.
//
// Success codes should be stable once published. Changing an existing success
// code is considered a breaking API contract change.
type SuccessCode string

const (
	// SuccessCodeDecisionCreated indicates that a decision was created.
	SuccessCodeDecisionCreated SuccessCode = "DECISION_CREATED"

	// SuccessCodeCustomerInitialCreditLimitAssigned indicates that initial customer credit limit assignment completed.
	SuccessCodeCustomerInitialCreditLimitAssigned SuccessCode = "CUSTOMER_INITIAL_CREDIT_LIMIT_ASSIGNED"

	// SuccessCodeDecisionExpired indicates that a decision was expired.
	SuccessCodeDecisionExpired SuccessCode = "DECISION_EXPIRED"

	// SuccessCodeDecisionOverridden indicates that a decision was overridden.
	SuccessCodeDecisionOverridden SuccessCode = "DECISION_OVERRIDDEN"

	// SuccessCodeContractCreated indicates that a contract was created.
	SuccessCodeContractCreated SuccessCode = "CONTRACT_CREATED"

	// SuccessCodeContractActivated indicates that a contract was activated.
	SuccessCodeContractActivated SuccessCode = "CONTRACT_ACTIVATED"

	// SuccessCodeRepaymentPlanCreated indicates that a repayment plan was created.
	SuccessCodeRepaymentPlanCreated SuccessCode = "REPAYMENT_PLAN_CREATED"

	// SuccessCodeInstallmentsListed indicates that installments were listed.
	SuccessCodeInstallmentsListed SuccessCode = "INSTALLMENTS_LISTED"

	// SuccessCodeUnpaidInstallmentsListed indicates that unpaid installments were listed.
	SuccessCodeUnpaidInstallmentsListed SuccessCode = "UNPAID_INSTALLMENTS_LISTED"

	// SuccessCodeActiveContractsListed indicates that active contracts were listed.
	SuccessCodeActiveContractsListed SuccessCode = "ACTIVE_CONTRACTS_LISTED"

	// SuccessCodeMerchantSalesListed indicates that merchant active sales were listed.
	SuccessCodeMerchantSalesListed SuccessCode = "MERCHANT_SALES_LISTED"

	// SuccessCodeMerchantBranchesListed indicates that merchant branches were listed.
	SuccessCodeMerchantBranchesListed SuccessCode = "MERCHANT_BRANCHES_LISTED"

	// SuccessCodeMerchantProfileRetrieved indicates that the merchant profile was returned.
	SuccessCodeMerchantProfileRetrieved SuccessCode = "MERCHANT_PROFILE_RETRIEVED"

	// SuccessCodeInstallmentPaymentsListed indicates that installment payment history was listed.
	SuccessCodeInstallmentPaymentsListed SuccessCode = "INSTALLMENT_PAYMENTS_LISTED"

	// SuccessCodeCustomerCreditRetrieved indicates that the customer credit summary was returned.
	SuccessCodeCustomerCreditRetrieved SuccessCode = "CUSTOMER_CREDIT_RETRIEVED"

	// SuccessCodeCustomerCreditExpired indicates that the customer has a prior credit
	// limit that is no longer usable and reassessment is required.
	SuccessCodeCustomerCreditExpired SuccessCode = "CUSTOMER_CREDIT_EXPIRED"

	// SuccessCodeCustomerCreditNotAssigned indicates that no credit limit has been
	// assigned yet and initial credit assessment is required.
	SuccessCodeCustomerCreditNotAssigned SuccessCode = "CUSTOMER_CREDIT_NOT_ASSIGNED"

	// SuccessCodePaymentIntentCreated indicates that a payment intent was created.
	SuccessCodePaymentIntentCreated SuccessCode = "PAYMENT_INTENT_CREATED"

	// SuccessCodeContractCancellationStarted indicates that contract cancellation
	// was accepted and the contract is canceling.
	SuccessCodeContractCancellationStarted SuccessCode = "CONTRACT_CANCELLATION_STARTED"

	// SuccessCodeCheckoutSessionCreated indicates that a merchant checkout session was created or replayed.
	SuccessCodeCheckoutSessionCreated SuccessCode = "CHECKOUT_SESSION_CREATED"

	// SuccessCodeCheckoutSessionRetrieved indicates that merchant-safe checkout state was returned.
	SuccessCodeCheckoutSessionRetrieved SuccessCode = "CHECKOUT_SESSION_RETRIEVED"

	// SuccessCodeCheckoutSessionStarted indicates that a verified customer checkout was evaluated.
	SuccessCodeCheckoutSessionStarted SuccessCode = "CHECKOUT_SESSION_STARTED"

	// SuccessCodeCheckoutSessionCanceled indicates that an eligible checkout session was canceled.
	SuccessCodeCheckoutSessionCanceled SuccessCode = "CHECKOUT_SESSION_CANCELED"

	// SuccessCodeCheckoutSessionConfirmed indicates that Confirm & Pay linked a contract.
	SuccessCodeCheckoutSessionConfirmed SuccessCode = "CHECKOUT_SESSION_CONFIRMED"

	// SuccessCodeCheckoutSessionReconfirmRequired indicates the presented offer was stale and a refreshed offer must be reconfirmed.
	SuccessCodeCheckoutSessionReconfirmRequired SuccessCode = "CHECKOUT_SESSION_RECONFIRM_REQUIRED"

	// SuccessCodeCheckoutSessionReturnRetrieved indicates the customer browser-return projection was returned.
	SuccessCodeCheckoutSessionReturnRetrieved SuccessCode = "CHECKOUT_SESSION_RETURN_RETRIEVED"

	// SuccessCodeCheckoutConfirmationStatusRetrieved indicates the hosted
	// confirmation-status poll projection was returned.
	SuccessCodeCheckoutConfirmationStatusRetrieved SuccessCode = "CHECKOUT_CONFIRMATION_STATUS_RETRIEVED"

	// SuccessCodeCheckoutOTPRequested indicates a hosted OTP was issued or an
	// active challenge was returned idempotently.
	SuccessCodeCheckoutOTPRequested SuccessCode = "CHECKOUT_OTP_REQUESTED"

	// SuccessCodeMerchantReturnDestinationUpserted indicates a return destination was saved.
	SuccessCodeMerchantReturnDestinationUpserted SuccessCode = "MERCHANT_RETURN_DESTINATION_UPSERTED"

	// SuccessCodeMerchantWebhookDestinationUpserted indicates a webhook destination was saved.
	SuccessCodeMerchantWebhookDestinationUpserted SuccessCode = "MERCHANT_WEBHOOK_DESTINATION_UPSERTED"

	// SuccessCodeMerchantTerminalProvisioned indicates a partner terminal and its first API key were issued.
	SuccessCodeMerchantTerminalProvisioned SuccessCode = "MERCHANT_TERMINAL_PROVISIONED"

	// SuccessCodeMerchantTerminalRetrieved indicates partner terminals were listed
	// for the authenticated merchant (possibly an empty collection).
	SuccessCodeMerchantTerminalRetrieved SuccessCode = "MERCHANT_TERMINAL_RETRIEVED"

	// SuccessCodeMerchantTerminalCredentialRotated indicates a partner terminal API key was rotated.
	SuccessCodeMerchantTerminalCredentialRotated SuccessCode = "MERCHANT_TERMINAL_CREDENTIAL_ROTATED"

	// SuccessCodeMerchantTerminalCredentialRetired indicates a retiring partner terminal credential was retired.
	SuccessCodeMerchantTerminalCredentialRetired SuccessCode = "MERCHANT_TERMINAL_CREDENTIAL_RETIRED"

	// SuccessCodeHealthOK indicates that a health check completed successfully.
	SuccessCodeHealthOK SuccessCode = "HEALTH_OK"
)

// SuccessMessageKey is an opaque reference to a human-readable success
// message in the i18n catalog. The underlying key is intentionally hidden so
// REST handlers use the predefined semantic messages below instead of passing
// raw user-facing strings.
type SuccessMessageKey struct {
	value string
}

// String returns the catalog key consumed by the configured i18n provider.
func (k SuccessMessageKey) String() string {
	return k.value
}

var (
	// SuccessMessageDefault is the generic successful-request message.
	SuccessMessageDefault = SuccessMessageKey{value: "success.default"}
	// SuccessMessageActiveContractsListed reports a successful active-contract listing.
	SuccessMessageActiveContractsListed = SuccessMessageKey{value: "success.active_contracts_listed"}
	// SuccessMessageCustomerCreditNotAssigned reports that credit assessment is required.
	SuccessMessageCustomerCreditNotAssigned = SuccessMessageKey{value: "success.customer_credit_has_not_been_assigned_credit_assessment_required"}
	// SuccessMessageCustomerCreditExpired reports that reassessment is required.
	SuccessMessageCustomerCreditExpired = SuccessMessageKey{value: "success.customer_credit_limit_expired_reassessment_required"}
	// SuccessMessageCustomerCreditSummaryReturned reports a successful credit-summary lookup.
	SuccessMessageCustomerCreditSummaryReturned = SuccessMessageKey{value: "success.customer_credit_summary_returned"}
	// SuccessMessageCheckoutDecisionAlreadyExists reports an idempotently reused checkout decision.
	SuccessMessageCheckoutDecisionAlreadyExists = SuccessMessageKey{value: "success.checkout_decision_already_exists"}
	// SuccessMessageCheckoutDecisionCreated reports successful checkout decision creation.
	SuccessMessageCheckoutDecisionCreated = SuccessMessageKey{value: "success.checkout_decision_created"}
	// SuccessMessageCheckoutSessionCreated reports a created or replayed checkout session.
	SuccessMessageCheckoutSessionCreated = SuccessMessageKey{value: "success.checkout_session_created"}
	// SuccessMessageCheckoutSessionRetrieved reports a successful merchant checkout status lookup.
	SuccessMessageCheckoutSessionRetrieved = SuccessMessageKey{value: "success.checkout_session_retrieved"}
	// SuccessMessageCheckoutSessionStarted reports a successful customer checkout evaluation.
	SuccessMessageCheckoutSessionStarted = SuccessMessageKey{value: "success.checkout_session_started"}
	// SuccessMessageCheckoutSessionCanceled reports a successful checkout cancellation.
	SuccessMessageCheckoutSessionCanceled = SuccessMessageKey{value: "success.checkout_session_canceled"}
	// SuccessMessageCheckoutSessionConfirmed reports a successful Confirm & Pay.
	SuccessMessageCheckoutSessionConfirmed = SuccessMessageKey{value: "success.checkout_session_confirmed"}
	// SuccessMessageCheckoutSessionReconfirmRequired reports a stale offer that needs reconfirmation.
	SuccessMessageCheckoutSessionReconfirmRequired = SuccessMessageKey{value: "success.checkout_session_reconfirm_required"}
	// SuccessMessageCheckoutSessionReturnRetrieved reports a successful browser-return lookup.
	SuccessMessageCheckoutSessionReturnRetrieved = SuccessMessageKey{value: "success.checkout_session_return_retrieved"}
	// SuccessMessageCheckoutConfirmationStatusRetrieved reports a successful
	// hosted confirmation-status poll.
	SuccessMessageCheckoutConfirmationStatusRetrieved = SuccessMessageKey{value: "success.checkout_confirmation_status_retrieved"}
	// SuccessMessageCheckoutOTPRequested reports a successful hosted OTP
	// issuance (or idempotent active challenge).
	SuccessMessageCheckoutOTPRequested = SuccessMessageKey{value: "success.checkout_otp_requested"}
	// SuccessMessageMerchantReturnDestinationUpserted reports a saved return destination.
	SuccessMessageMerchantReturnDestinationUpserted = SuccessMessageKey{value: "success.merchant_return_destination_upserted"}
	// SuccessMessageMerchantWebhookDestinationUpserted reports a saved webhook destination.
	SuccessMessageMerchantWebhookDestinationUpserted = SuccessMessageKey{value: "success.merchant_webhook_destination_upserted"}
	// SuccessMessageMerchantTerminalProvisioned reports an issued partner terminal.
	SuccessMessageMerchantTerminalProvisioned = SuccessMessageKey{value: "success.merchant_terminal_provisioned"}
	// SuccessMessageMerchantTerminalReturned reports listed partner terminals
	// (including an empty list when none are provisioned).
	SuccessMessageMerchantTerminalReturned = SuccessMessageKey{value: "success.merchant_terminal_returned"}
	// SuccessMessageMerchantTerminalCredentialRotated reports a rotated partner API key.
	SuccessMessageMerchantTerminalCredentialRotated = SuccessMessageKey{value: "success.merchant_terminal_credential_rotated"}
	// SuccessMessageMerchantTerminalCredentialRetired reports a retired partner credential.
	SuccessMessageMerchantTerminalCredentialRetired = SuccessMessageKey{value: "success.merchant_terminal_credential_retired"}
	// SuccessMessageContractInstallmentsListed reports a successful installment listing.
	SuccessMessageContractInstallmentsListed = SuccessMessageKey{value: "success.contract_installments_listed"}
	// SuccessMessageInitialCustomerCreditLimitAssigned reports successful initial-limit assignment.
	SuccessMessageInitialCustomerCreditLimitAssigned = SuccessMessageKey{value: "success.initial_customer_credit_limit_assigned"}
	// SuccessMessageInitialCustomerCreditLimitAssignmentDenied reports a denied initial-limit assignment.
	SuccessMessageInitialCustomerCreditLimitAssignmentDenied = SuccessMessageKey{value: "success.initial_customer_credit_limit_assignment_was_denied"}
	// SuccessMessageInitialCustomerCreditLimitAlreadyActive reports an idempotently active initial limit.
	SuccessMessageInitialCustomerCreditLimitAlreadyActive = SuccessMessageKey{value: "success.initial_customer_credit_limit_is_already_active"}
	// SuccessMessageInstallmentPaymentAttemptAlreadyExists reports an idempotently reused payment attempt.
	SuccessMessageInstallmentPaymentAttemptAlreadyExists = SuccessMessageKey{value: "success.installment_payment_attempt_already_exists"}
	// SuccessMessageInstallmentPaymentAttemptQueued reports successful payment-attempt queuing.
	SuccessMessageInstallmentPaymentAttemptQueued = SuccessMessageKey{value: "success.installment_payment_attempt_queued"}
	// SuccessMessageInstallmentPaymentHistoryListed reports a successful payment-history listing.
	SuccessMessageInstallmentPaymentHistoryListed = SuccessMessageKey{value: "success.installment_payment_history_listed"}
	// SuccessMessageMerchantBranchesListed reports a successful merchant-branch listing.
	SuccessMessageMerchantBranchesListed = SuccessMessageKey{value: "success.merchant_branches_listed"}
	// SuccessMessageMerchantProfileReturned reports a successful merchant-profile lookup.
	SuccessMessageMerchantProfileReturned = SuccessMessageKey{value: "success.merchant_profile_returned"}
	// SuccessMessageMerchantSalesListed reports a successful merchant-sales listing.
	SuccessMessageMerchantSalesListed = SuccessMessageKey{value: "success.merchant_sales_listed"}
	// SuccessMessageContractAlreadyExists reports an idempotently reused contract.
	SuccessMessageContractAlreadyExists = SuccessMessageKey{value: "success.contract_already_exists"}
	// SuccessMessageContractCreated reports successful contract creation.
	SuccessMessageContractCreated = SuccessMessageKey{value: "success.contract_created"}
	// SuccessMessageContractCancellationStarted reports that cancellation was accepted.
	SuccessMessageContractCancellationStarted = SuccessMessageKey{value: "success.contract_cancellation_started"}
	// SuccessMessageContractCancellationAlreadyExists reports an idempotent cancellation replay.
	SuccessMessageContractCancellationAlreadyExists = SuccessMessageKey{value: "success.contract_cancellation_already_exists"}
	// SuccessMessageUnpaidInstallmentsListed reports a successful unpaid-installment listing.
	SuccessMessageUnpaidInstallmentsListed = SuccessMessageKey{value: "success.unpaid_installments_listed"}
)

// SuccessResponse is the platform-wide successful response envelope.
//
// Every 2xx REST response should use this structure.
//
// Data contains the endpoint-specific response payload. It may be nil, in which
// case it is serialized as JSON null.
//
// Meta contains standard response metadata such as status, stable response code,
// HTTP status code, timestamps, trace identifiers, and optional pagination.
type SuccessResponse struct {
	Data any         `json:"data"`
	Meta SuccessMeta `json:"meta"`
}

// SuccessMeta carries standard metadata for successful REST responses.
//
// Status is always the canonical machine-readable value "success" and is never localized.
//
// Code is a stable machine-readable success identifier and is never localized.
//
// Message is human-readable and may change over time. Clients should not use it
// as a stable contract.
//
// StatusCode mirrors the HTTP response status code for clients that process the
// body independently of the HTTP layer.
//
// Timestamp is generated in UTC using RFC3339 format.
//
// TraceID, RequestID, and CorrelationID are used for observability and request
// correlation.
//
// Pagination is populated only for list endpoints.
//
// Non-list OpenAPI success envelopes should reference SuccessMetaNoPagination
// (and keyset lists SuccessMetaKeyset) so Example Value matches the wire shape.
type SuccessMeta struct {
	Status        string          `json:"status" example:"success" enums:"success" i18n:"canonical"`
	Code          string          `json:"code" example:"MERCHANT_SALES_LISTED" i18n:"canonical"`
	Message       string          `json:"message" example:"Merchant sales listed." i18n:"localized"`
	StatusCode    int             `json:"status_code" example:"200"`
	Timestamp     string          `json:"timestamp" example:"2026-08-09T12:00:00Z"`
	TraceID       string          `json:"trace_id" example:"a1b2c3d4-e5f6-7890-abcd-ef1234567890" format:"uuid"`
	RequestID     string          `json:"request_id" example:"b2c3d4e5-f6a7-8901-bcde-f12345678901" format:"uuid"`
	CorrelationID *string         `json:"correlation_id,omitempty" example:"c3d4e5f6-a7b8-9012-cdef-123456789012" format:"uuid"`
	Pagination    *PaginationMeta `json:"pagination,omitempty"`
}

// PaginationMeta describes pagination state for list responses.
//
// Exactly one of Page, Offset, Cursor, or Keyset should be populated according
// to Type. OpenAPI documents all variants; runtime responses populate only the
// variant matching Type.
type PaginationMeta struct {
	Type        string      `json:"type" example:"keyset" enums:"page,offset,cursor,keyset"`
	Returned    int         `json:"returned" example:"20"`
	HasNext     bool        `json:"has_next" example:"true"`
	HasPrevious bool        `json:"has_previous" example:"false"`
	Page        *PageInfo   `json:"page,omitempty"`
	Offset      *OffsetInfo `json:"offset,omitempty"`
	Cursor      *CursorInfo `json:"cursor,omitempty"`
	Keyset      *KeysetInfo `json:"keyset,omitempty"`
}

// PageInfo is populated when PaginationMeta.Type is "page".
type PageInfo struct {
	CurrentPage int   `json:"current_page"`
	PageSize    int   `json:"page_size"`
	TotalItems  int64 `json:"total_items"`
	TotalPages  int   `json:"total_pages"`
}

// OffsetInfo is populated when PaginationMeta.Type is "offset".
type OffsetInfo struct {
	Offset     int    `json:"offset"`
	Limit      int    `json:"limit"`
	TotalItems *int64 `json:"total_items,omitempty"`
}

// CursorInfo is populated when PaginationMeta.Type is "cursor".
type CursorInfo struct {
	Limit          int     `json:"limit"`
	NextCursor     *string `json:"next_cursor,omitempty"`
	PreviousCursor *string `json:"previous_cursor,omitempty"`
}

// KeysetInfo is populated when PaginationMeta.Type is "keyset".
type KeysetInfo struct {
	Limit              int     `json:"limit"`
	NextAfterID        *string `json:"next_after_id,omitempty"`
	NextAfterCreatedAt *string `json:"next_after_created_at,omitempty"`
	NextAfterDueAt     *string `json:"next_after_due_at,omitempty"`
}

// buildMeta constructs SuccessMeta from the request context.
func buildMeta(
	c *gin.Context,
	statusCode int,
	code SuccessCode,
	messageKey SuccessMessageKey,
	pagination *PaginationMeta,
) SuccessMeta {
	ctx := c.Request.Context()

	var correlationID *string
	if cid := trace.CorrelationID(ctx); cid != "" {
		correlationID = &cid
	}

	return SuccessMeta{
		Status:        "success",
		Code:          string(code),
		Message:       localizeSuccess(ctx, messageKey),
		StatusCode:    statusCode,
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		TraceID:       trace.ID(ctx),
		RequestID:     trace.RequestID(ctx),
		CorrelationID: correlationID,
		Pagination:    pagination,
	}
}

// Ser writes a non-paginated successful REST response.
func Ser(
	c *gin.Context,
	statusCode int,
	code SuccessCode,
	messageKey SuccessMessageKey,
	data any,
) {
	c.JSON(statusCode, SuccessResponse{
		Data: data,
		Meta: buildMeta(
			c,
			statusCode,
			code,
			messageKey,
			nil,
		),
	})
}

// SerPage writes a page-number paginated successful REST response.
func SerPage[T any](
	c *gin.Context,
	statusCode int,
	code SuccessCode,
	messageKey SuccessMessageKey,
	page corepagination.OffsetPage[T],
) {
	m := page.Meta

	pagination := &PaginationMeta{
		Type:        "page",
		Returned:    len(page.Data),
		HasNext:     m.HasNext,
		HasPrevious: m.HasPrevious,
		Page: &PageInfo{
			CurrentPage: m.Page,
			PageSize:    m.Limit,
			TotalItems:  m.Total,
			TotalPages:  m.TotalPages,
		},
	}

	c.JSON(statusCode, SuccessResponse{
		Data: page.Data,
		Meta: buildMeta(
			c,
			statusCode,
			code,
			messageKey,
			pagination,
		),
	})
}

// SerCursor writes a cursor-paginated successful REST response.
func SerCursor[T any](
	c *gin.Context,
	statusCode int,
	code SuccessCode,
	messageKey SuccessMessageKey,
	page corepagination.CursorPage[T],
) {
	m := page.Meta

	pagination := &PaginationMeta{
		Type:     "cursor",
		Returned: len(page.Data),
		HasNext:  m.HasNext,
		Cursor: &CursorInfo{
			Limit:      m.Limit,
			NextCursor: m.NextCursor,
		},
	}

	c.JSON(statusCode, SuccessResponse{
		Data: page.Data,
		Meta: buildMeta(
			c,
			statusCode,
			code,
			messageKey,
			pagination,
		),
	})
}

// SerKeyset writes a keyset-paginated successful REST response.
func SerKeyset[T any](
	c *gin.Context,
	statusCode int,
	code SuccessCode,
	messageKey SuccessMessageKey,
	page corepagination.KeysetPage[T],
) {
	m := page.Meta

	pagination := &PaginationMeta{
		Type:     "keyset",
		Returned: len(page.Data),
		HasNext:  m.HasNext,
		Keyset: &KeysetInfo{
			Limit:              m.Limit,
			NextAfterID:        m.NextAfterID,
			NextAfterCreatedAt: m.NextAfterCreatedAt,
			NextAfterDueAt:     m.NextAfterDueAt,
		},
	}

	c.JSON(statusCode, SuccessResponse{
		Data: page.Data,
		Meta: buildMeta(
			c,
			statusCode,
			code,
			messageKey,
			pagination,
		),
	})
}
