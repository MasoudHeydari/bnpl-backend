package noop

import (
	"context"

	coreerrorreporting "github.com/parstis/bnpl-backend/pkg/core/errorreporting"
)

// Reporter discards every reported event.
type Reporter struct{}

// New creates a no-op reporter.
func New() *Reporter {
	return &Reporter{}
}

// CaptureError intentionally discards err and metadata.
func (r *Reporter) CaptureError(context.Context, error, coreerrorreporting.Metadata) {}

// CapturePanic intentionally discards the recovered value and metadata.
func (r *Reporter) CapturePanic(context.Context, any, coreerrorreporting.Metadata) {}

// Flush always succeeds because the noop reporter has no buffered events.
func (r *Reporter) Flush(context.Context) bool { return true }

// Close has no resources to release.
func (r *Reporter) Close() error { return nil }
