package returndestination

import (
	"errors"
	"regexp"
)

const (
	// MaxDestinationNameLength is the maximum length of a symbolic return
	// destination name. This is the single Go source of truth for the bound.
	MaxDestinationNameLength = 64

	// DestinationNamePattern is the canonical destination-name grammar shared by
	// Core validation, OpenAPI documentation, and the PostgreSQL CHECK constraint
	// (PostgreSQL uses the equivalent form without a non-capturing group).
	DestinationNamePattern = `^[a-z0-9]+(-[a-z0-9]+)*$`
)

var (
	// ErrDestinationNameEmpty means the destination name is missing.
	ErrDestinationNameEmpty = errors.New("return destination name is required")

	// ErrDestinationNameTooLong means the destination name exceeds MaxDestinationNameLength.
	ErrDestinationNameTooLong = errors.New("return destination name is too long")

	// ErrDestinationNameInvalid means the destination name violates the
	// lowercase kebab-case grammar.
	ErrDestinationNameInvalid = errors.New("return destination name is invalid")

	// destinationNamePattern is the compiled DestinationNamePattern.
	// Lowercase ASCII letters, digits, and single hyphens; no leading/trailing
	// hyphen; no consecutive hyphens.
	destinationNamePattern = regexp.MustCompile(DestinationNamePattern)
)

// ValidateDestinationName reports whether name is a canonical symbolic return
// destination identifier.
//
// The value is validated as provided: whitespace is rejected, and uppercase is
// not silently lowercased.
func ValidateDestinationName(name string) error {
	if name == "" {
		return ErrDestinationNameEmpty
	}

	if len(name) > MaxDestinationNameLength {
		return ErrDestinationNameTooLong
	}

	if !destinationNamePattern.MatchString(name) {
		return ErrDestinationNameInvalid
	}

	return nil
}
