package trace

import (
	"crypto/rand"
	"time"

	"github.com/google/uuid"
	"github.com/oklog/ulid/v2"

	"github.com/parstis/bnpl-backend/pkg/core/identifier"
	coretrace "github.com/parstis/bnpl-backend/pkg/core/trace"
)

// Verify Provider implements the core trace provider contract.
var _ coretrace.Provider = (*Provider)(nil)

// Verify Provider implements the core trace ID generator contract.
var _ coretrace.IDGenerator = (*Provider)(nil)

// Verify Provider implements the generic identifier generator contract.
var _ identifier.Generator = (*Provider)(nil)

// Provider generates transport tracing identifiers.
//
// Trace and request IDs use ULID (compact, sortable, log-friendly). Correlation
// IDs use UUID so minted X-Correlation-Id values match common cross-service
// expectations. The core layer treats all of them as opaque strings.
type Provider struct{}

// NewULIDProvider returns the default transport trace provider.
func NewULIDProvider() *Provider {
	return &Provider{}
}

// NewTraceID returns a new trace ID.
func (p *Provider) NewTraceID() string {
	return newULID()
}

// NewRequestID returns a new request ID.
func (p *Provider) NewRequestID() string {
	return newULID()
}

// NewCorrelationID returns a new correlation ID.
func (p *Provider) NewCorrelationID() string {
	return uuid.NewString()
}

// Generate returns a new opaque identifier for use-case level event generation.
func (p *Provider) Generate() string {
	return newULID()
}

func newULID() string {
	return ulid.MustNew(ulid.Timestamp(time.Now()), rand.Reader).String()
}
