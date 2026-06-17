package sarama

import (
	"context"
	"fmt"

	"github.com/IBM/sarama"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
)

// HealthChecker verifies the availability of the Kafka cluster.
type HealthChecker struct {
	client sarama.Client
}

// NewHealthChecker constructs a Kafka messaging health checker.
func NewHealthChecker(client sarama.Client) *HealthChecker {
	return &HealthChecker{
		client: client,
	}
}

// Ping verifies that the Kafka cluster is reachable.
func (h *HealthChecker) Ping(context.Context) error {
	_, err := h.client.Controller()
	if err != nil {
		return cerr.ServiceUnavailable(fmt.Errorf("kafka: %w", err))
	}

	return nil
}

// IsMessaging marks the implementation as a messaging health checker.
func (h *HealthChecker) IsMessaging() {}

// Close releases the underlying Kafka client and any resources it owns.
func (h *HealthChecker) Close() error {
	if h.client == nil {
		return nil
	}

	return h.client.Close()
}
