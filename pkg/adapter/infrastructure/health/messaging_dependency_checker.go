package health

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/health"
	"github.com/parstis/bnpl-backend/pkg/core/messaging"
)

// Verify MessagingDependencyChecker implements the core dependency checker
// contract.
var _ health.DependencyChecker = (*MessagingDependencyChecker)(nil)

// MessagingDependencyChecker verifies that the application's Kafka cluster is
// available.
//
// The checker performs a lightweight connectivity check against the configured
// messaging backend. If no messaging backend is configured, the check is
// treated as successful.
type MessagingDependencyChecker struct {
	health messaging.Health
}

// NewMessagingDependencyChecker constructs a Kafka dependency checker.
func NewMessagingDependencyChecker(
	health messaging.Health,
) *MessagingDependencyChecker {
	return &MessagingDependencyChecker{
		health: health,
	}
}

// Check verifies that Kafka is reachable and able to serve lightweight
// requests.
func (c *MessagingDependencyChecker) Check(ctx context.Context) error {
	if c.health == nil {
		return nil
	}

	if err := c.health.Ping(ctx); err != nil {
		return cerr.ServiceUnavailable(fmt.Errorf("kafka: %w", err))
	}

	return nil
}
