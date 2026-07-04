package customercredituc

import (
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// InitialLimitAssessmentInput carries customer profile context for initial limit assessment.
type InitialLimitAssessmentInput struct {
	// CustomerID is the internal customer_profiles.id primary key.
	CustomerID model.CustomerID
	// IdentitySubjectID is the Identity Service subject mapped to customer_profiles.identity_subject_id.
	IdentitySubjectID model.IdentitySubjectID
	// Jurisdiction is the operating jurisdiction.
	Jurisdiction model.Jurisdiction

	// RequestedAt is when the assessment was requested.
	RequestedAt time.Time
}

// InitialLimitAssessmentResult carries assessed initial limit decision material.
type InitialLimitAssessmentResult struct {
	// Outcome is the assessed decision outcome.
	Outcome model.DecisionOutcome

	// AmountMinor is the amount in minor currency units.
	AmountMinor uint64
	// Currency is the ISO currency for monetary amounts.
	Currency model.Currency
	// ValidFor is how long an approved limit remains valid from assignment.
	ValidFor time.Duration

	// PolicyID identifies the assessment policy.
	PolicyID string
	// PolicyVersion is the assessment policy version.
	PolicyVersion string
	// ReasonCodes lists policy reason codes for the assessment.
	ReasonCodes []string
}
