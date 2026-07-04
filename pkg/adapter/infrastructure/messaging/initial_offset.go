package messaging

import (
	"fmt"
	"strings"
)

// InitialOffset identifies where a new consumer group begins reading when no
// committed offset exists yet.
//
// This is operational messaging configuration, not a BNPL domain concept. Broker
// adapters map these values to provider-specific constants.
type InitialOffset string

// Supported initial-offset policies for new consumer groups.
const (
	InitialOffsetOldest InitialOffset = "oldest"
	InitialOffsetNewest InitialOffset = "newest"
)

// ParseInitialOffset validates and normalizes an initial-offset policy.
//
// An empty value defaults to InitialOffsetOldest. Invalid values return an error
// and are never silently remapped.
func ParseInitialOffset(value string) (InitialOffset, error) {
	const op = "messaging.ParseInitialOffset"

	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return InitialOffsetOldest, nil
	}

	offset := InitialOffset(normalized)
	if !offset.IsValid() {
		return "", fmt.Errorf("%s: unsupported initial offset %q", op, value)
	}

	return offset, nil
}

// IsValid reports whether the initial-offset policy is supported.
func (o InitialOffset) IsValid() bool {
	switch o {
	case InitialOffsetOldest, InitialOffsetNewest:
		return true

	default:
		return false
	}
}
