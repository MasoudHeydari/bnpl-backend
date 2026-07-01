package messaging

import "context"

// Health verifies that the messaging backend is reachable.
type Health interface {
	// Ping verifies that the messaging backend is reachable and able to serve
	// lightweight requests.
	Ping(ctx context.Context) error

	// Close releases any resources owned by the messaging health checker.
	//
	// Implementations that do not own any resources should return nil.
	Close() error

	// IsMessaging marks the implementation as a messaging health checker.
	//
	// The method exists solely to prevent unrelated types with matching method
	// signatures from satisfying this interface accidentally.
	IsMessaging()
}
