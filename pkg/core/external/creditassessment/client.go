package creditassessment

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// Client fetches external credit assessment signals for internal policy evaluation.
type Client interface {
	// FetchSignals retrieves normalized external credit signals for the customer context.
	FetchSignals(ctx context.Context, input FetchSignalsInput) (FetchSignalsResult, error)
}

// FetchSignalsInput identifies the customer context for external signal retrieval.
type FetchSignalsInput struct {
	// CustomerID is the internal BNPL customer primary key.
	CustomerID model.CustomerID
	// IdentitySubjectID is the external Identity Service subject identifier.
	IdentitySubjectID model.IdentitySubjectID
	// Jurisdiction is the regulatory jurisdiction governing the assessment.
	Jurisdiction model.Jurisdiction
	// RequestedAt is when the signal fetch was requested.
	RequestedAt time.Time
}

// FetchSignalsResult carries normalized external credit signals.
type FetchSignalsResult struct {
	// Provider identifies the external credit assessment provider.
	Provider string
	// ProviderReferenceID is the provider's reference for this fetch, when available.
	ProviderReferenceID *string

	// FetchedAt is when the signals were retrieved.
	FetchedAt time.Time
	// ExpiresAt is when the retrieved signals should be considered stale.
	ExpiresAt time.Time

	// Signals are the normalized credit assessment signals from the provider.
	Signals []Signal
}

// Signal is a normalized external credit assessment signal.
type Signal struct {
	// Code is the machine-readable signal code.
	Code string
	// Value is the signal value as a normalized string.
	Value string
	// Source identifies where the signal originated within the provider response.
	Source string
	// ObservedAt is when the underlying observation was made, if known.
	ObservedAt *time.Time
	// ExpiresAt is when this individual signal expires, if known.
	ExpiresAt *time.Time
}
