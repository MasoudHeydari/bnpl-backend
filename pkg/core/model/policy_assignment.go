package model

import (
	"database/sql/driver"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// PolicyAssignmentID uniquely identifies a policy assignment history row.
type PolicyAssignmentID uuid.UUID

// ParsePolicyAssignmentID parses a policy assignment id from its canonical string form.
func ParsePolicyAssignmentID(value string) (PolicyAssignmentID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return PolicyAssignmentID{}, fmt.Errorf("parse policy assignment id: %w", err)
	}

	return PolicyAssignmentID(id), nil
}

func (id PolicyAssignmentID) UUID() uuid.UUID { return uuid.UUID(id) }

func (id PolicyAssignmentID) String() string { return uuid.UUID(id).String() }

func (id PolicyAssignmentID) IsZero() bool { return uuid.UUID(id) == uuid.Nil }

func (id PolicyAssignmentID) MarshalText() ([]byte, error) { return uuid.UUID(id).MarshalText() }

func (id *PolicyAssignmentID) UnmarshalText(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data); err != nil {
		return err
	}

	*id = PolicyAssignmentID(u)

	return nil
}

func (id PolicyAssignmentID) Value() (driver.Value, error) { return uuid.UUID(id).Value() }

func (id *PolicyAssignmentID) Scan(src any) error {
	var u uuid.UUID
	if err := u.Scan(src); err != nil {
		return err
	}

	*id = PolicyAssignmentID(u)

	return nil
}

// PolicyAssignmentImportID uniquely identifies one durable bulk-import execution.
type PolicyAssignmentImportID uuid.UUID

// ParsePolicyAssignmentImportID parses a policy assignment import id from its canonical string form.
func ParsePolicyAssignmentImportID(value string) (PolicyAssignmentImportID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return PolicyAssignmentImportID{}, fmt.Errorf("parse policy assignment import id: %w", err)
	}

	return PolicyAssignmentImportID(id), nil
}

func (id PolicyAssignmentImportID) UUID() uuid.UUID { return uuid.UUID(id) }

func (id PolicyAssignmentImportID) String() string { return uuid.UUID(id).String() }

func (id PolicyAssignmentImportID) IsZero() bool { return uuid.UUID(id) == uuid.Nil }

func (id PolicyAssignmentImportID) MarshalText() ([]byte, error) { return uuid.UUID(id).MarshalText() }

func (id *PolicyAssignmentImportID) UnmarshalText(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data); err != nil {
		return err
	}

	*id = PolicyAssignmentImportID(u)

	return nil
}

func (id PolicyAssignmentImportID) Value() (driver.Value, error) { return uuid.UUID(id).Value() }

func (id *PolicyAssignmentImportID) Scan(src any) error {
	var u uuid.UUID
	if err := u.Scan(src); err != nil {
		return err
	}

	*id = PolicyAssignmentImportID(u)

	return nil
}

// PolicyAssignmentImportItemID uniquely identifies one row in a durable import.
type PolicyAssignmentImportItemID uuid.UUID

// ParsePolicyAssignmentImportItemID parses a policy assignment import item id from its canonical string form.
func ParsePolicyAssignmentImportItemID(value string) (PolicyAssignmentImportItemID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return PolicyAssignmentImportItemID{}, fmt.Errorf("parse policy assignment import item id: %w", err)
	}

	return PolicyAssignmentImportItemID(id), nil
}

func (id PolicyAssignmentImportItemID) UUID() uuid.UUID { return uuid.UUID(id) }

func (id PolicyAssignmentImportItemID) String() string { return uuid.UUID(id).String() }

func (id PolicyAssignmentImportItemID) IsZero() bool { return uuid.UUID(id) == uuid.Nil }

func (id PolicyAssignmentImportItemID) MarshalText() ([]byte, error) {
	return uuid.UUID(id).MarshalText()
}

func (id *PolicyAssignmentImportItemID) UnmarshalText(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data); err != nil {
		return err
	}

	*id = PolicyAssignmentImportItemID(u)

	return nil
}

func (id PolicyAssignmentImportItemID) Value() (driver.Value, error) { return uuid.UUID(id).Value() }

func (id *PolicyAssignmentImportItemID) Scan(src any) error {
	var u uuid.UUID
	if err := u.Scan(src); err != nil {
		return err
	}

	*id = PolicyAssignmentImportItemID(u)

	return nil
}

// PolicySubjectType identifies the entity kind receiving a policy assignment.
type PolicySubjectType string

const (
	// CustomerPolicySubjectType assigns policy behavior to an Identity Service customer subject.
	CustomerPolicySubjectType PolicySubjectType = "customer"
)

// PolicyFamily identifies an independently versioned policy family.
type PolicyFamily string

const (
	CheckoutPolicyFamily           PolicyFamily = "checkout"
	InitialCreditLimitPolicyFamily PolicyFamily = "initial_credit_limit"
)

// PolicyAssignment stores one immutable interval in policy-assignment history.
// Active assignment selection uses ValidFrom/ValidUntil; superseding an
// assignment closes the prior interval instead of deleting history.
type PolicyAssignment struct {
	ID            PolicyAssignmentID `db:"id"`
	SubjectType   PolicySubjectType  `db:"subject_type"`
	SubjectID     string             `db:"subject_id"`
	PolicyFamily  PolicyFamily       `db:"policy_family"`
	PolicyID      string             `db:"policy_id"`
	PolicyVersion string             `db:"policy_version"`
	Reason        string             `db:"reason"`
	Source        string             `db:"source"`
	ValidFrom     time.Time          `db:"valid_from"`
	ValidUntil    *time.Time         `db:"valid_until"`
	CreatedBy     string             `db:"created_by"`
	CreatedAt     time.Time          `db:"created_at"`
	UpdatedAt     time.Time          `db:"updated_at"`
}

// PolicyAssignmentImportStatus identifies durable batch-import progress.
type PolicyAssignmentImportStatus string

const (
	ProcessingPolicyAssignmentImportStatus            PolicyAssignmentImportStatus = "processing"
	CompletedPolicyAssignmentImportStatus             PolicyAssignmentImportStatus = "completed"
	CompletedWithFailuresPolicyAssignmentImportStatus PolicyAssignmentImportStatus = "completed_with_failures"
)

// PolicyAssignmentImport records one bulk assignment import invocation.
type PolicyAssignmentImport struct {
	ID           PolicyAssignmentImportID     `db:"id"`
	ImportType   string                       `db:"import_type"`
	SourceFile   string                       `db:"source_file"`
	Status       PolicyAssignmentImportStatus `db:"status"`
	TotalCount   int                          `db:"total_count"`
	SuccessCount int                          `db:"success_count"`
	FailureCount int                          `db:"failure_count"`
	CreatedBy    string                       `db:"created_by"`
	CreatedAt    time.Time                    `db:"created_at"`
	CompletedAt  *time.Time                   `db:"completed_at"`
}

// PolicyAssignmentImportItemStatus identifies the per-subject processing result.
type PolicyAssignmentImportItemStatus string

const (
	PendingPolicyAssignmentImportItemStatus   PolicyAssignmentImportItemStatus = "pending"
	SucceededPolicyAssignmentImportItemStatus PolicyAssignmentImportItemStatus = "succeeded"
	FailedPolicyAssignmentImportItemStatus    PolicyAssignmentImportItemStatus = "failed"
)

// PolicyAssignmentImportItem stores the durable result for one CSV row.
type PolicyAssignmentImportItem struct {
	ID           PolicyAssignmentImportItemID     `db:"id"`
	ImportID     PolicyAssignmentImportID         `db:"import_id"`
	RowNumber    int                              `db:"row_number"`
	SubjectID    string                           `db:"subject_id"`
	Status       PolicyAssignmentImportItemStatus `db:"status"`
	ErrorCode    *string                          `db:"error_code"`
	ErrorMessage *string                          `db:"error_message"`
	CreatedAt    time.Time                        `db:"created_at"`
	UpdatedAt    time.Time                        `db:"updated_at"`
}

// ValidatePolicyAssignmentSubjectID validates the generic persisted subject identity.
func ValidatePolicyAssignmentSubjectID(subjectID string) error {
	if subjectID == "" {
		return fmt.Errorf("policy assignment subject id is required")
	}

	if len(subjectID) > 256 {
		return fmt.Errorf("policy assignment subject id is too long")
	}

	return nil
}
