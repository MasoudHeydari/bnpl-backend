package model

import (
	"database/sql/driver"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// CreditRequestID uniquely identifies a durable credit request.
type CreditRequestID uuid.UUID

// ParseCreditRequestID parses a CreditRequestID from its canonical string form.
func ParseCreditRequestID(value string) (CreditRequestID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return CreditRequestID{}, fmt.Errorf("parse credit request id: %w", err)
	}

	return CreditRequestID(id), nil
}

func (id CreditRequestID) UUID() uuid.UUID { return uuid.UUID(id) }

func (id CreditRequestID) String() string { return uuid.UUID(id).String() }

func (id CreditRequestID) IsZero() bool { return uuid.UUID(id) == uuid.Nil }

func (id CreditRequestID) MarshalText() ([]byte, error) { return uuid.UUID(id).MarshalText() }

func (id *CreditRequestID) UnmarshalText(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data); err != nil {
		return err
	}

	*id = CreditRequestID(u)

	return nil
}

func (id *CreditRequestID) Scan(src any) error {
	var u uuid.UUID
	if err := u.Scan(src); err != nil {
		return err
	}

	*id = CreditRequestID(u)

	return nil
}

func (id CreditRequestID) Value() (driver.Value, error) { return uuid.UUID(id).Value() }

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
	ID                 CreditRequestID           `db:"id"`
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
	AllocationRuleID   *CreditAllocationRuleID   `db:"allocation_rule_id"`
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
	ID          CreditRequestID     `json:"id" swaggertype:"string" format:"uuid"`
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
