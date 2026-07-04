package customer

import "fmt"

// Status is a Customer Reference Service customer status wire value.
type Status string

const (
	// ActiveStatus is a usable Customer Reference Service customer.
	ActiveStatus Status = "active"

	// SuspendedStatus is a temporarily unusable Customer Reference Service
	// customer.
	SuspendedStatus Status = "suspended"
)

// ParseStatus maps a Customer Reference Service status string.
//
// Unsupported values (including a historical Identity "deleted" status) fail
// closed; callers must not invent missing statuses.
func ParseStatus(raw string) (Status, error) {
	switch Status(raw) {
	case ActiveStatus, SuspendedStatus:
		return Status(raw), nil

	default:
		return "", fmt.Errorf("unknown customer reference status %q", raw)
	}
}
