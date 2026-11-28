package model

import (
	"database/sql/driver"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// MerchantParticipationRequestID is the Merchant Service reference.
type MerchantParticipationRequestID uuid.UUID

// ParseMerchantParticipationRequestID parses a MerchantParticipationRequestID from its canonical string form.
func ParseMerchantParticipationRequestID(value string) (MerchantParticipationRequestID, error) {
	const op = "model.ParseMerchantParticipationRequestID"

	id, err := uuid.Parse(value)
	if err != nil {
		return MerchantParticipationRequestID{}, fmt.Errorf("%s: %w", op, err)
	}

	return MerchantParticipationRequestID(id), nil
}

func (id MerchantParticipationRequestID) UUID() uuid.UUID { return uuid.UUID(id) }

func (id MerchantParticipationRequestID) String() string { return uuid.UUID(id).String() }

func (id MerchantParticipationRequestID) IsZero() bool { return uuid.UUID(id) == uuid.Nil }

func (id MerchantParticipationRequestID) MarshalText() ([]byte, error) {
	return uuid.UUID(id).MarshalText()
}

func (id *MerchantParticipationRequestID) UnmarshalText(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data); err != nil {
		return err
	}

	*id = MerchantParticipationRequestID(u)

	return nil
}

func (id *MerchantParticipationRequestID) Scan(src any) error {
	var u uuid.UUID
	if err := u.Scan(src); err != nil {
		return err
	}

	*id = MerchantParticipationRequestID(u)

	return nil
}

func (id MerchantParticipationRequestID) Value() (driver.Value, error) { return uuid.UUID(id).Value() }

// MerchantApplicationID is the Merchant Service reference.
type MerchantApplicationID uuid.UUID

// ParseMerchantApplicationID parses a MerchantApplicationID from its canonical string form.
func ParseMerchantApplicationID(value string) (MerchantApplicationID, error) {
	const op = "model.ParseMerchantApplicationID"

	id, err := uuid.Parse(value)
	if err != nil {
		return MerchantApplicationID{}, fmt.Errorf("%s: %w", op, err)
	}

	return MerchantApplicationID(id), nil
}

func (id MerchantApplicationID) UUID() uuid.UUID { return uuid.UUID(id) }

func (id MerchantApplicationID) String() string { return uuid.UUID(id).String() }

func (id MerchantApplicationID) IsZero() bool { return uuid.UUID(id) == uuid.Nil }

func (id MerchantApplicationID) MarshalText() ([]byte, error) { return uuid.UUID(id).MarshalText() }

func (id *MerchantApplicationID) UnmarshalText(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data); err != nil {
		return err
	}

	*id = MerchantApplicationID(u)

	return nil
}

func (id *MerchantApplicationID) Scan(src any) error {
	var u uuid.UUID
	if err := u.Scan(src); err != nil {
		return err
	}

	*id = MerchantApplicationID(u)

	return nil
}

func (id MerchantApplicationID) Value() (driver.Value, error) { return uuid.UUID(id).Value() }

// MerchantParticipationIssue is a non-sensitive, stable declaration validation result.
type MerchantParticipationIssue struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

const (
	MerchantParticipationReady            = "READY"
	MerchantParticipationChangesRequested = "CHANGES_REQUESTED"
)

// MerchantParticipation records one immutable evaluation and its durable report.
// Delivery state is independent of account readiness.
type MerchantParticipation struct {
	RequestID           MerchantParticipationRequestID `db:"request_id"`
	ApplicationID       MerchantApplicationID          `db:"application_id"`
	MerchantID          ExternalMerchantID             `db:"external_merchant_id"`
	ProfileID           *MerchantProfileID             `db:"merchant_profile_id"`
	Environment         string                         `db:"environment"`
	EventID             string                         `db:"progress_event_id"`
	OwnerVersion        int64                          `db:"owner_version"`
	DeclarationRevision int64                          `db:"declaration_revision"`
	PolicyID            string                         `db:"policy_id"`
	PolicyVersion       string                         `db:"policy_version"`
	ProgressStatus      string                         `db:"progress_status"`
	Issues              []MerchantParticipationIssue   `db:"issues"`
	DeliveryStatus      string                         `db:"delivery_status"`
	AttemptCount        int64                          `db:"attempt_count"`
	NextAttemptAt       time.Time                      `db:"next_attempt_at"`
}

// MatchesCommand compares only the immutable business identity of a request.
func (p MerchantParticipation) MatchesCommand(merchant ExternalMerchantID, application MerchantApplicationID, environment string) bool {
	return p.MerchantID == merchant && p.ApplicationID == application && p.Environment == environment
}
