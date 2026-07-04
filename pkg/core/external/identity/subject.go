package identity

import "fmt"

// SubjectType is an Identity Service subject type wire value.
type SubjectType string

const (
	// IndividualSubjectType is a natural-person Identity subject.
	IndividualSubjectType SubjectType = "individual"

	// GroupSubjectType is a group Identity subject. Valid on the Identity wire
	// contract, but unsupported by BNPL customer checkout today.
	GroupSubjectType SubjectType = "group"
)

// ParseSubjectType maps an Identity wire subject_type string.
func ParseSubjectType(raw string) (SubjectType, error) {
	switch SubjectType(raw) {
	case IndividualSubjectType, GroupSubjectType:
		return SubjectType(raw), nil

	default:
		return "", fmt.Errorf("unknown identity subject_type %q", raw)
	}
}

// SubjectStatus is an Identity Service subject status wire value.
type SubjectStatus string

const (
	// ActiveSubjectStatus is a usable Identity subject.
	ActiveSubjectStatus SubjectStatus = "active"

	// SuspendedSubjectStatus is a temporarily unusable Identity subject.
	SuspendedSubjectStatus SubjectStatus = "suspended"

	// DeletedSubjectStatus is a permanently unusable Identity subject.
	DeletedSubjectStatus SubjectStatus = "deleted"
)

// ParseSubjectStatus maps an Identity wire subject_status string.
func ParseSubjectStatus(raw string) (SubjectStatus, error) {
	switch SubjectStatus(raw) {
	case ActiveSubjectStatus, SuspendedSubjectStatus, DeletedSubjectStatus:
		return SubjectStatus(raw), nil

	default:
		return "", fmt.Errorf("unknown identity subject_status %q", raw)
	}
}
