package outboxpublisheruc

import (
	"testing"

	"github.com/parstis/bnpl-backend/pkg/core/mock"
)

// newTestObserver returns an OutboxPublisherObserver without default
// expectations so individual tests can assert exact checkpoints.
func newTestObserver(t *testing.T) *mock.OutboxPublisherObserver {
	t.Helper()

	return mock.NewOutboxPublisherObserver(t)
}
