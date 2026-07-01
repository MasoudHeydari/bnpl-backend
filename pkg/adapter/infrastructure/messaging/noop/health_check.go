package noop

import (
	"context"
)

// HealthChecker provides a no-op messaging health checker.
type HealthChecker struct{}

// NewHealthChecker constructs a no-op messaging health checker.
func NewHealthChecker() *HealthChecker {
	return &HealthChecker{}
}

// Ping always succeeds because the noop implementation has no external
// dependency.
func (h *HealthChecker) Ping(context.Context) error {
	return nil
}

// IsMessaging marks the implementation as a messaging health checker.
func (h *HealthChecker) IsMessaging() {}

// Close releases any resources owned by the messaging health checker.
//
// The noop implementation does not own any resources, so Close always returns nil.
func (h *HealthChecker) Close() error {
	return nil
}
