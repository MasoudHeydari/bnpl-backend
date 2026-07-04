package uuidgen

import (
	"github.com/google/uuid"

	"github.com/parstis/bnpl-backend/pkg/core/identifier"
)

// Provider generates UUID identifiers for UUID-backed domain entities.
type Provider struct{}

// New returns a UUID identifier generator.
func New() *Provider {
	return &Provider{}
}

var _ identifier.Generator = (*Provider)(nil)

// Generate returns a new random UUID string suitable for uuid.Parse.
func (*Provider) Generate() string {
	return uuid.NewString()
}
