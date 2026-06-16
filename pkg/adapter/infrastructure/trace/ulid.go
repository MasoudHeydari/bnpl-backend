package trace

import (
	"crypto/rand"
	"time"

	"github.com/oklog/ulid/v2"
	coretrace "github.com/parstis/bnpl-backend/pkg/core/trace"
)

// Verify Provider implements the core trace provider contract.
var _ coretrace.Provider = (*Provider)(nil)

// Provider generates trace identifiers using ULID.
//
// ULIDs are compact, sortable, log-friendly identifiers. The core layer treats
// them as opaque strings and does not depend on the ULID format.
type Provider struct{}

// NewULIDProvider returns a ULID-backed trace provider.
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
	return newULID()
}

func newULID() string {
	return ulid.MustNew(ulid.Timestamp(time.Now()), rand.Reader).String()
}
