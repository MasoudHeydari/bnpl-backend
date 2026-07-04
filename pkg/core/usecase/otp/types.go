package otp

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// IssuanceAuthorizer validates whether a new OTP may be issued for the current
// application context under locks already held by the caller-supplied Tx.
//
// Implementations must not perform network I/O. They may lock and inspect
// application rows inside tx.
type IssuanceAuthorizer interface {
	AuthorizeIssuance(ctx context.Context, tx repo.Tx) (AuthorizedIssuance, error)
}

// ConsumeAuthorizer optionally re-validates application eligibility after a
// correct code hash match and before MarkVerified. Returning an error aborts
// the transaction without consuming the challenge so a concurrent business
// blocker (for example saga manual_review) cannot burn a usable OTP.
//
// Wrong-code paths must not invoke ConsumeAuthorizer. Implementations must not
// perform network I/O.
type ConsumeAuthorizer interface {
	AuthorizeConsume(ctx context.Context, tx repo.Tx) error
}

// AuthorizedIssuance is the trusted delivery and rate-limit material returned
// after application eligibility succeeds.
type AuthorizedIssuance struct {
	// Purpose is the OTP purpose being authorized.
	Purpose model.OTPPurpose

	// SubjectKey is the opaque subject identity for the purpose.
	SubjectKey string

	// ContextVersion is the opaque application binding for this issuance.
	ContextVersion string

	// Binding is included in DomainSeparatedMaterial (for hosted checkout, the
	// public checkout token text).
	Binding string

	// Recipient is the canonical notification destination (E.164 for SMS).
	Recipient string

	// TemplateID selects the Notification template.
	TemplateID string

	// Variables are template placeholders. Values may carry secrets.
	Variables map[string]string

	// IdempotencyKeyPrefix namespaces the Notification idempotency key; the
	// use case appends ":" + issueID.
	IdempotencyKeyPrefix string

	// CorrelationID is tracing-only and is not part of idempotency.
	CorrelationID string

	// RateLimits are cross-subject budgets consumed atomically with Issue.
	RateLimits []RateLimitBudget

	// Terminal reports whether the application context is terminal for
	// availability projection after issuance.
	Terminal bool

	// BusinessBlocked reports whether issuance/entry is blocked by application
	// rules even when a cryptographically usable challenge exists.
	BusinessBlocked bool
}

// RateLimitBudget is one cross-subject fixed-window send budget.
type RateLimitBudget struct {
	Scope    model.OTPRateLimitScope
	ScopeKey string
	Window   time.Duration
	MaxSends int
}

// RequestInput requests an OTP for a purpose/subject/context.
type RequestInput struct {
	Purpose        model.OTPPurpose
	SubjectKey     string
	ContextVersion string
	Authorizer     IssuanceAuthorizer
}

// RequestOutput is the privacy-safe OTP issuance result.
type RequestOutput struct {
	Challenge      model.OTPChallenge
	OTPSent        bool
	OTPExpiresAt   *time.Time
	OTPNextIssueAt *time.Time
	Availability   string
}

// VerifyInput verifies and consumes an OTP for a purpose/subject/context.
type VerifyInput struct {
	Purpose        model.OTPPurpose
	SubjectKey     string
	ContextVersion string
	Binding        string
	Code           string

	// ConsumeAuthorizer, when non-nil, runs after a successful hash match and
	// before MarkVerified. On error VerifyAndConsumeInTx returns
	// (VerifyOutput{}, nil, err) so the transaction rolls back without
	// consuming. Wrong-code paths never call it.
	ConsumeAuthorizer ConsumeAuthorizer
}

// VerifyOutput is the trusted result of a successful OTP consumption.
type VerifyOutput struct {
	Challenge model.OTPChallenge
}

// AvailabilityInput projects privacy-safe OTP countdown / actionability state.
// Terminal and BusinessBlocked are supplied by the calling application so this
// package does not re-implement business eligibility.
//
// ContextVersion is the caller's current opaque binding (for hosted checkout,
// the live evaluation generation). A challenge bound to a different version
// must not project as awaiting_entry.
type AvailabilityInput struct {
	Purpose         model.OTPPurpose
	SubjectKey      string
	ContextVersion  string
	Terminal        bool
	BusinessBlocked bool
}

// AvailabilityOutput is a read-only OTP availability snapshot.
type AvailabilityOutput struct {
	Challenge      model.OTPChallenge
	Found          bool
	OTPExpiresAt   *time.Time
	OTPNextIssueAt *time.Time
	Availability   string
}
