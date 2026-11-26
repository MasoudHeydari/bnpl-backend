package model

import (
	"time"

	"github.com/google/uuid"
)

// CreditRequestStatus represents the persisted request, not the current
// account balance or a Financial Core monetary obligation.
type CreditRequestStatus string

const (
	PendingCreditRequestStatus    CreditRequestStatus = "pending"
	ProcessingCreditRequestStatus CreditRequestStatus = "processing"
	ApprovedCreditRequestStatus   CreditRequestStatus = "approved"
	RejectedCreditRequestStatus   CreditRequestStatus = "rejected"
	CanceledCreditRequestStatus   CreditRequestStatus = "canceled"
)

type CreditRequestQueueSource string

const (
	NewCreditRequestQueueSource             CreditRequestQueueSource = "new_request"
	EligibilityCreditRequestQueueSource     CreditRequestQueueSource = "eligibility"
	PublicCreditRequestQueueSource          CreditRequestQueueSource = "public"
	IdentityRecheckCreditRequestQueueSource CreditRequestQueueSource = "identity_recheck"
)

const CreditRequestIdentityMismatchReason = "CREDIT_REQUEST_IDENTITY_MISMATCH"

// CreditRequest retains the immutable identity snapshot from submission.
// Never replace it with a later mobile or expose it in a customer response.
// A lease fences workers after crashes; only its holder can finalize a grant.
type CreditRequest struct {
	ID                 uuid.UUID                 `db:"id"`
	CreditOwnerID      CreditOwnerID             `db:"credit_owner_id"`
	CustomerID         CustomerID                `db:"customer_id"`
	ExternalCustomerID ExternalCustomerID        `db:"external_customer_id"`
	MobileE164         string                    `db:"mobile_e164"`
	NationalCode       string                    `db:"national_code"`
	CreditKind         CreditKind                `db:"credit_kind"`
	Currency           Currency                  `db:"currency"`
	Status             CreditRequestStatus       `db:"status"`
	ReasonCode         string                    `db:"reason_code"`
	DecisionID         *DecisionID               `db:"decision_id"`
	AllocationRuleID   *uuid.UUID                `db:"allocation_rule_id"`
	QueueSource        *CreditRequestQueueSource `db:"queue_source"`
	NextAttemptAt      *time.Time                `db:"next_attempt_at"`
	LeaseToken         *uuid.UUID                `db:"lease_token"`
	LeaseUntil         *time.Time                `db:"lease_until"`
	RequestedAt        time.Time                 `db:"requested_at"`
	UpdatedAt          time.Time                 `db:"updated_at"`
	CompletedAt        *time.Time                `db:"completed_at"`
}

// CreditRequestSummary deliberately omits identity, queue and rule data.
type CreditRequestSummary struct {
	ID          uuid.UUID           `json:"id" swaggertype:"string" format:"uuid"`
	Status      CreditRequestStatus `json:"status" enums:"pending,processing,approved,rejected,canceled"`
	ReasonCode  string              `json:"reason_code,omitempty"`
	RequestedAt time.Time           `json:"requested_at"`
}

func (r CreditRequest) Summary() CreditRequestSummary {
	// Internal eligibility and dependency reasons are never shown as denial.
	reason := ""
	if r.Status == RejectedCreditRequestStatus {
		reason = r.ReasonCode
	}
	return CreditRequestSummary{ID: r.ID, Status: r.Status, ReasonCode: reason, RequestedAt: r.RequestedAt}
}
