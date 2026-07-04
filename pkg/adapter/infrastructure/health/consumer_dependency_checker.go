package health

import (
	"context"
	"errors"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	corehealth "github.com/parstis/bnpl-backend/pkg/core/health"
)

var _ corehealth.DependencyChecker = (*ConsumerDependencyChecker)(nil)

// ConsumerDependencyChecker verifies Kafka consumer runtime supervision state.
type ConsumerDependencyChecker struct {
	consumerHealth corehealth.ConsumerHealth
}

// NewConsumerDependencyChecker constructs a consumer supervision checker.
func NewConsumerDependencyChecker(consumerHealth corehealth.ConsumerHealth) *ConsumerDependencyChecker {
	return &ConsumerDependencyChecker{consumerHealth: consumerHealth}
}

// Check verifies that the message consumer is healthy.
func (c *ConsumerDependencyChecker) Check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if c.consumerHealth == nil {
		return nil
	}

	if !c.consumerHealth.IsReady() {
		return cerr.ServiceUnavailable(errors.New("message consumer is not ready"))
	}

	return nil
}
