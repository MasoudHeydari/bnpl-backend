package model

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	// ErrInvalidDecisionID is returned when the decision ID is empty.
	ErrInvalidDecisionID = errors.New("invalid decision id")

	// ErrInvalidDecisionTargetID is returned when the decision target ID is empty.
	ErrInvalidDecisionTargetID = errors.New("invalid decision target id")

	// ErrInvalidDecisionType is returned when an unknown decision type is used.
	ErrInvalidDecisionType = errors.New("invalid decision type")

	// ErrInvalidDecisionTargetType is returned when an unknown decision target type is used.
	ErrInvalidDecisionTargetType = errors.New("invalid decision target type")

	// ErrInvalidDecisionOutcome is returned when an unknown decision outcome is used.
	ErrInvalidDecisionOutcome = errors.New("invalid decision outcome")

	// ErrInvalidDecisionActorType is returned when an unknown decision actor type is used.
	ErrInvalidDecisionActorType = errors.New("invalid decision actor type")

	// ErrDecisionActorIDRequired is returned when an admin decision has no actor ID.
	ErrDecisionActorIDRequired = errors.New("decision actor id required for admin decisions")

	// ErrIncompatibleDecisionOutcome is returned when the outcome is not valid for the decision type.
	ErrIncompatibleDecisionOutcome = errors.New("decision outcome is incompatible with decision type")
)

// DecisionType identifies the business meaning of a governed BNPL decision.
//
// A decision type describes what kind of business question was answered.
// It does not describe the target entity. For example, EligibilityDecisionType
// may apply to a borrower profile, merchant profile, or merchant branch profile.
type DecisionType string

const (
	// EligibilityDecisionType determines whether a target may participate in
	// BNPL at the profile or configuration level.
	EligibilityDecisionType DecisionType = "eligibility"

	// CreditLimitDecisionType determines or changes the maximum BNPL exposure
	// allowed for a target.
	//
	// The approved amount itself must be stored in a specialized model such as
	// BorrowerCreditLimit, MerchantCreditLimit, or BranchCreditLimit.
	CreditLimitDecisionType DecisionType = "credit_limit"

	// RestrictionDecisionType applies, changes, or removes a BNPL restriction
	// for a target.
	RestrictionDecisionType DecisionType = "restriction"

	// ContractApprovalDecisionType determines whether a specific BNPL contract
	// may be created or activated.
	ContractApprovalDecisionType DecisionType = "contract_approval"

	// ManualOverrideDecisionType records a governed manual override of a prior
	// decision.
	ManualOverrideDecisionType DecisionType = "manual_override"
)

// IsValid reports whether the decision type is known by the domain.
func (t DecisionType) IsValid() bool {
	switch t {
	case EligibilityDecisionType,
		CreditLimitDecisionType,
		RestrictionDecisionType,
		ContractApprovalDecisionType,
		ManualOverrideDecisionType:
		return true
	default:
		return false
	}
}

// allowsOutcome reports whether the outcome is compatible with the decision type.
func (t DecisionType) allowsOutcome(o DecisionOutcome) bool {
	switch t {
	case EligibilityDecisionType, ContractApprovalDecisionType:
		switch o {
		case ApprovedDecisionOutcome,
			DeniedDecisionOutcome,
			ConditionalDecisionOutcome:
			return true
		default:
			return false
		}
	case CreditLimitDecisionType:
		switch o {
		case LimitSetDecisionOutcome,
			DeniedDecisionOutcome,
			ConditionalDecisionOutcome:
			return true
		default:
			return false
		}
	case RestrictionDecisionType:
		switch o {
		case RestrictedDecisionOutcome,
			UnrestrictedDecisionOutcome:
			return true
		default:
			return false
		}
	case ManualOverrideDecisionType:
		// A governed manual override may produce any valid outcome.
		return o.IsValid()
	default:
		return false
	}
}

// DecisionTargetType identifies the entity type a decision applies to.
type DecisionTargetType string

const (
	// BorrowerProfileDecisionTargetType indicates the decision applies to a
	// borrower's BNPL profile.
	BorrowerProfileDecisionTargetType DecisionTargetType = "borrower_profile"

	// MerchantProfileDecisionTargetType indicates the decision applies to a
	// merchant's BNPL profile.
	MerchantProfileDecisionTargetType DecisionTargetType = "merchant_profile"

	// MerchantBranchProfileDecisionTargetType indicates the decision applies to
	// a merchant branch BNPL profile.
	MerchantBranchProfileDecisionTargetType DecisionTargetType = "merchant_branch_profile"

	// ContractDecisionTargetType indicates the decision applies to a BNPL
	// contract.
	ContractDecisionTargetType DecisionTargetType = "contract"

	// CheckoutIntentDecisionTargetType indicates the decision applies to a
	// checkout intent before contract creation.
	CheckoutIntentDecisionTargetType DecisionTargetType = "checkout_intent"
)

// IsValid reports whether the decision target type is known by the domain.
func (t DecisionTargetType) IsValid() bool {
	switch t {
	case BorrowerProfileDecisionTargetType,
		MerchantProfileDecisionTargetType,
		MerchantBranchProfileDecisionTargetType,
		ContractDecisionTargetType,
		CheckoutIntentDecisionTargetType:
		return true
	default:
		return false
	}
}

// DecisionOutcome identifies the final result of a governed BNPL decision.
type DecisionOutcome string

const (
	// ApprovedDecisionOutcome indicates the requested decision target was
	// approved.
	ApprovedDecisionOutcome DecisionOutcome = "approved"

	// DeniedDecisionOutcome indicates the requested decision target was denied.
	DeniedDecisionOutcome DecisionOutcome = "denied"

	// ConditionalDecisionOutcome indicates approval depends on explicit
	// conditions or constraints.
	ConditionalDecisionOutcome DecisionOutcome = "conditional"

	// LimitSetDecisionOutcome indicates a credit limit was assigned or updated.
	LimitSetDecisionOutcome DecisionOutcome = "limit_set"

	// RestrictedDecisionOutcome indicates a restriction was applied.
	RestrictedDecisionOutcome DecisionOutcome = "restricted"

	// UnrestrictedDecisionOutcome indicates a restriction was removed.
	UnrestrictedDecisionOutcome DecisionOutcome = "unrestricted"
)

// IsValid reports whether the decision outcome is known by the domain.
func (o DecisionOutcome) IsValid() bool {
	switch o {
	case ApprovedDecisionOutcome,
		DeniedDecisionOutcome,
		ConditionalDecisionOutcome,
		LimitSetDecisionOutcome,
		RestrictedDecisionOutcome,
		UnrestrictedDecisionOutcome:
		return true
	default:
		return false
	}
}

// DecisionActorType identifies who or what produced a decision.
type DecisionActorType string

const (
	// SystemDecisionActorType indicates the decision was produced by automated
	// system policy evaluation.
	SystemDecisionActorType DecisionActorType = "system"

	// AdminDecisionActorType indicates the decision was produced by an
	// authorized administrative operator.
	AdminDecisionActorType DecisionActorType = "admin"
)

// IsValid reports whether the decision actor type is known by the domain.
func (a DecisionActorType) IsValid() bool {
	switch a {
	case SystemDecisionActorType,
		AdminDecisionActorType:
		return true
	default:
		return false
	}
}

// Decision represents a governed BNPL decision.
//
// A Decision is an immutable audit artifact that explains why a material
// outcome was approved, denied, restricted, overridden, or assigned.
//
// This model stores the common decision envelope only. Specialized outputs must
// be stored in their own models and reference this record by DecisionID.
//
// Examples:
//   - BorrowerCreditLimit stores the approved amount and references DecisionID.
//   - BorrowerRestriction stores the applied restriction and references DecisionID.
//   - Contract stores or references the approval decision used for activation.
type Decision struct {
	// ID uniquely identifies this decision.
	ID uuid.UUID `db:"id" json:"id"`

	// Type identifies the business question answered by this decision.
	Type DecisionType `db:"type" json:"type,omitempty"`

	// TargetType identifies the kind of entity this decision applies to.
	TargetType DecisionTargetType `db:"target_type" json:"target_type,omitempty"`

	// TargetID identifies the specific entity this decision applies to.
	TargetID uuid.UUID `db:"target_id" json:"target_id"`

	// Outcome is the final result of the decision.
	Outcome DecisionOutcome `db:"outcome" json:"outcome,omitempty"`

	// ReasonCodes stores stable, machine-readable explanation codes.
	//
	// These codes must come from a controlled catalog and must not be free-text
	// explanations. User-facing messages should be derived from these codes.
	ReasonCodes []string `db:"reason_codes" json:"reason_codes,omitempty"`

	// PolicyID identifies the policy used to evaluate this decision.
	PolicyID string `db:"policy_id" json:"policy_id,omitempty"`

	// PolicyVersion identifies the exact policy version used to evaluate this
	// decision.
	PolicyVersion string `db:"policy_version" json:"policy_version,omitempty"`

	// Jurisdiction identifies the legal or regulatory scope used for this
	// decision.
	Jurisdiction Jurisdiction `db:"jurisdiction" json:"jurisdiction,omitempty"`

	// ActorType identifies whether the decision was produced by the system or by
	// an authorized admin.
	ActorType DecisionActorType `db:"actor_type" json:"actor_type,omitempty"`

	// ActorID identifies the admin or system actor that produced the decision.
	//
	// It may be nil for automated system decisions when the system actor is
	// represented only by ActorType.
	ActorID *uuid.UUID `db:"actor_id" json:"actor_id,omitempty"`

	// PriorDecisionID references the previous decision when this decision is an
	// override, renewal, or correction.
	//
	// The previous decision must not be mutated. A new decision must be created
	// instead.
	PriorDecisionID *uuid.UUID `db:"prior_decision_id" json:"prior_decision_id,omitempty"`

	// ReviewableUntil indicates the latest time this decision can be reviewed or
	// challenged, when applicable.
	ReviewableUntil *time.Time `db:"reviewable_until" json:"reviewable_until,omitempty"`

	// ExpiresAt indicates when this decision stops being valid for future use.
	//
	// Expiration does not delete or mutate the decision. It only prevents future
	// reliance on it.
	ExpiresAt *time.Time `db:"expires_at" json:"expires_at,omitempty"`

	// DecidedAt is the authoritative time when the decision was made.
	DecidedAt time.Time `db:"decided_at" json:"decided_at"`

	// CreatedAt is when the decision record was persisted.
	CreatedAt time.Time `db:"created_at" json:"created_at"`

	// IdempotencyKey identifies a client-supplied idempotency token when present.
	IdempotencyKey string `db:"idempotency_key" json:"idempotency_key,omitempty"`

	// IdempotencyFingerprint stores the material checkout inputs used to produce
	// this decision for idempotency conflict detection.
	IdempotencyFingerprint string `db:"idempotency_fingerprint" json:"idempotency_fingerprint,omitempty"`
}

// Validate verifies that the decision envelope is internally consistent.
func (d *Decision) Validate() error {
	if d.ID == uuid.Nil {
		return ErrInvalidDecisionID
	}

	if d.TargetID == uuid.Nil {
		return ErrInvalidDecisionTargetID
	}

	if !d.Type.IsValid() {
		return ErrInvalidDecisionType
	}

	if !d.TargetType.IsValid() {
		return ErrInvalidDecisionTargetType
	}

	if !d.Outcome.IsValid() {
		return ErrInvalidDecisionOutcome
	}

	if !d.ActorType.IsValid() {
		return ErrInvalidDecisionActorType
	}

	if d.ActorType == AdminDecisionActorType && d.ActorID == nil {
		return ErrDecisionActorIDRequired
	}

	if !d.Type.allowsOutcome(d.Outcome) {
		return ErrIncompatibleDecisionOutcome
	}

	return nil
}
