package model

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// CustomerID is the internal BNPL primary key for a customer profile
// (customer_profiles.id). External callers never supply this value.
type CustomerID uuid.UUID

// ParseCustomerID parses a CustomerID from its canonical string form.
func ParseCustomerID(value string) (CustomerID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return CustomerID{}, fmt.Errorf("parse customer id: %w", err)
	}

	return CustomerID(id), nil
}

func (id CustomerID) UUID() uuid.UUID { return uuid.UUID(id) }

func (id CustomerID) String() string { return uuid.UUID(id).String() }

func (id CustomerID) IsZero() bool { return uuid.UUID(id) == uuid.Nil }

func (id CustomerID) MarshalText() ([]byte, error) { return uuid.UUID(id).MarshalText() }

func (id *CustomerID) UnmarshalText(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data); err != nil {
		return err
	}

	*id = CustomerID(u)

	return nil
}

func (id *CustomerID) Scan(src any) error {
	var u uuid.UUID
	if err := u.Scan(src); err != nil {
		return err
	}

	*id = CustomerID(u)

	return nil
}

func (id CustomerID) Value() (driver.Value, error) { return uuid.UUID(id).Value() }

// ExternalSubjectID is the Identity Service subject UUID stored as
// customer_profiles.external_subject_id (from x-auth-subject-id).
//
// It is never the BNPL-local customer_profiles.id (CustomerID) and never the
// Customer Reference Service customer_id (ExternalCustomerID).
type ExternalSubjectID uuid.UUID

// ParseExternalSubjectID parses an ExternalSubjectID from its canonical string form.
func ParseExternalSubjectID(value string) (ExternalSubjectID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return ExternalSubjectID{}, fmt.Errorf("parse external subject id: %w", err)
	}

	return ExternalSubjectID(id), nil
}

func (id ExternalSubjectID) UUID() uuid.UUID { return uuid.UUID(id) }

func (id ExternalSubjectID) String() string { return uuid.UUID(id).String() }

func (id ExternalSubjectID) IsZero() bool { return uuid.UUID(id) == uuid.Nil }

func (id ExternalSubjectID) MarshalText() ([]byte, error) { return uuid.UUID(id).MarshalText() }

func (id *ExternalSubjectID) UnmarshalText(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data); err != nil {
		return err
	}

	*id = ExternalSubjectID(u)

	return nil
}

func (id *ExternalSubjectID) Scan(src any) error {
	var u uuid.UUID
	if err := u.Scan(src); err != nil {
		return err
	}

	*id = ExternalSubjectID(u)

	return nil
}

func (id ExternalSubjectID) Value() (driver.Value, error) { return uuid.UUID(id).Value() }

// ExternalCustomerID is the external Customer Reference Service customer_id
// stored as customer_profiles.external_customer_id.
//
// It is never the BNPL-local customer_profiles.id (CustomerID) and never the
// Identity Service subject (ExternalSubjectID / x-auth-subject-id).
type ExternalCustomerID uuid.UUID

// ParseExternalCustomerID parses an ExternalCustomerID from its canonical string form.
func ParseExternalCustomerID(value string) (ExternalCustomerID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return ExternalCustomerID{}, fmt.Errorf("parse external customer id: %w", err)
	}

	return ExternalCustomerID(id), nil
}

func (id ExternalCustomerID) UUID() uuid.UUID { return uuid.UUID(id) }

func (id ExternalCustomerID) String() string { return uuid.UUID(id).String() }

func (id ExternalCustomerID) IsZero() bool { return uuid.UUID(id) == uuid.Nil }

func (id ExternalCustomerID) MarshalText() ([]byte, error) { return uuid.UUID(id).MarshalText() }

func (id *ExternalCustomerID) UnmarshalText(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data); err != nil {
		return err
	}

	*id = ExternalCustomerID(u)

	return nil
}

func (id *ExternalCustomerID) Scan(src any) error {
	var u uuid.UUID
	if err := u.Scan(src); err != nil {
		return err
	}

	*id = ExternalCustomerID(u)

	return nil
}

func (id ExternalCustomerID) Value() (driver.Value, error) { return uuid.UUID(id).Value() }

var (
	// ErrInvalidCustomerID is returned when the internal BNPL customer primary key is empty.
	ErrInvalidCustomerID = errors.New("invalid customer id")

	// ErrInvalidExternalSubjectID is returned when the Identity Service subject id is empty.
	ErrInvalidExternalSubjectID = errors.New("invalid external subject id")

	// ErrInvalidExternalCustomerID is returned when the Customer Reference Service customer id is empty.
	ErrInvalidExternalCustomerID = errors.New("invalid external customer id")

	// ErrInvalidCustomerStatus is returned when an unknown customer status is used.
	ErrInvalidCustomerStatus = errors.New("invalid customer status")

	// ErrInvalidCustomerJurisdiction is returned when jurisdiction is empty or invalid.
	ErrInvalidCustomerJurisdiction = errors.New("invalid customer jurisdiction")

	// ErrTerminatedCustomerProfile is returned when a terminal customer profile is changed.
	ErrTerminatedCustomerProfile = errors.New("terminated customer profile cannot be changed")
)

// CustomerStatus identifies the lifecycle state of a customer's BNPL profile.
//
// This status describes the profile lifecycle only. Risk, fraud, compliance,
// or collections restrictions must be modeled separately as governed
// restrictions backed by Decisions.
type CustomerStatus string

const (
	// ActiveCustomerStatus indicates the customer profile is active at the BNPL
	// profile level. Transaction-level eligibility still requires a Decision.
	ActiveCustomerStatus CustomerStatus = "active"

	// DeactivatedCustomerStatus indicates the customer profile was voluntarily
	// or administratively deactivated and cannot start new BNPL flows.
	DeactivatedCustomerStatus CustomerStatus = "deactivated"

	// TerminatedCustomerStatus indicates the customer relationship has ended
	// and the profile is retained only for audit and record-keeping.
	TerminatedCustomerStatus CustomerStatus = "terminated"
)

var customerStatusTransitions = newStatusTransitionTable(
	"customer status",
	map[CustomerStatus][]CustomerStatus{
		ActiveCustomerStatus: {
			DeactivatedCustomerStatus,
			TerminatedCustomerStatus,
		},
		DeactivatedCustomerStatus: {
			ActiveCustomerStatus,
			TerminatedCustomerStatus,
		},
	},
	[]CustomerStatus{
		TerminatedCustomerStatus,
	},
)

// IsValid reports whether the customer status is known by the domain.
func (s CustomerStatus) IsValid() bool {
	return customerStatusTransitions.isValid(s)
}

// CanTransitionTo reports whether the customer status may change to next.
func (s CustomerStatus) CanTransitionTo(next CustomerStatus) bool {
	return customerStatusTransitions.canTransitionTo(s, next)
}

// ValidateTransitionTo returns an error when the customer status may not change to next.
func (s CustomerStatus) ValidateTransitionTo(next CustomerStatus) error {
	return customerStatusTransitions.validateTransitionTo(s, next)
}

// IsTerminal reports whether the status is terminal.
func (s CustomerStatus) IsTerminal() bool {
	return customerStatusTransitions.isTerminal(s)
}

// IsActive reports whether the customer profile lifecycle is active.
func (s CustomerStatus) IsActive() bool {
	return s == ActiveCustomerStatus
}

// CustomerProfile represents BNPL-owned state for a customer. BNPL stores the
// Identity Service subject id, the Customer Reference Service customer_id, and
// local lifecycle state. Raw PII such as national ID remains outside BNPL.
type CustomerProfile struct {
	// ID is the internal BNPL primary key for this profile (customer_profiles.id).
	// It is generated by BNPL and must not be supplied by external callers.
	ID CustomerID `db:"id" json:"id"`

	// ExternalSubjectID is the Identity Service subject UUID
	// (customer_profiles.external_subject_id / x-auth-subject-id). It is not
	// equal to ID or ExternalCustomerID.
	ExternalSubjectID ExternalSubjectID `db:"external_subject_id" json:"external_subject_id"`

	// ExternalCustomerID is the Customer Reference Service customer_id
	// (customer_profiles.external_customer_id). It is not equal to ID or
	// ExternalSubjectID.
	ExternalCustomerID ExternalCustomerID `db:"external_customer_id" json:"external_customer_id"`

	// Jurisdiction identifies the regulatory jurisdiction that governs this
	// customer's BNPL profile.
	Jurisdiction Jurisdiction `db:"jurisdiction" json:"jurisdiction,omitempty"`

	// Status is the current lifecycle state of the BNPL profile.
	Status CustomerStatus `db:"status" json:"status,omitempty"`

	// CreatedAt is when the profile was created.
	CreatedAt time.Time `db:"created_at" json:"created_at"`

	// UpdatedAt is when the profile was last modified.
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// NewCustomerProfile creates a new customer BNPL profile in active state.
// Every customer identity received through the trusted authenticated gateway
// is considered valid; there is no local review or verification gate.
func NewCustomerProfile(
	id CustomerID,
	externalSubjectID ExternalSubjectID,
	externalCustomerID ExternalCustomerID,
	jurisdiction Jurisdiction,
	now time.Time,
) (CustomerProfile, error) {
	if id.IsZero() {
		return CustomerProfile{}, ErrInvalidCustomerID
	}

	if externalSubjectID.IsZero() {
		return CustomerProfile{}, ErrInvalidExternalSubjectID
	}

	if externalCustomerID.IsZero() {
		return CustomerProfile{}, ErrInvalidExternalCustomerID
	}

	if !jurisdiction.IsValid() {
		return CustomerProfile{}, ErrInvalidCustomerJurisdiction
	}

	return CustomerProfile{
		ID:                 id,
		ExternalSubjectID:  externalSubjectID,
		ExternalCustomerID: externalCustomerID,
		Jurisdiction:       jurisdiction,
		Status:             ActiveCustomerStatus,
		CreatedAt:          now,
		UpdatedAt:          now,
	}, nil
}

// Validate verifies that the customer profile is internally consistent.
func (p *CustomerProfile) Validate() error {
	if p.ID.IsZero() {
		return ErrInvalidCustomerID
	}

	if p.ExternalSubjectID.IsZero() {
		return ErrInvalidExternalSubjectID
	}

	if p.ExternalCustomerID.IsZero() {
		return ErrInvalidExternalCustomerID
	}

	if !p.Status.IsValid() {
		return ErrInvalidCustomerStatus
	}

	if !p.Jurisdiction.IsValid() {
		return ErrInvalidCustomerJurisdiction
	}

	return nil
}

// IsActive reports whether the customer profile lifecycle is active.
//
// This does not mean the customer is eligible for every BNPL transaction.
// Transaction approval must still be decided by eligibility and limit checks.
func (p *CustomerProfile) IsActive() bool {
	return p.Status == ActiveCustomerStatus
}

// Deactivate marks the customer profile as deactivated.
//
// Deactivation is reversible by a later governed activation.
func (p *CustomerProfile) Deactivate(now time.Time) error {
	if p.Status.IsTerminal() {
		return ErrTerminatedCustomerProfile
	}

	p.Status = DeactivatedCustomerStatus
	p.UpdatedAt = now

	return nil
}

// Terminate marks the customer profile as terminal.
//
// Terminated profiles are retained for audit and record-keeping and must not be
// reactivated.
func (p *CustomerProfile) Terminate(now time.Time) error {
	p.Status = TerminatedCustomerStatus
	p.UpdatedAt = now

	return nil
}
