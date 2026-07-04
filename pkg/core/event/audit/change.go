package audit

import "errors"

// FieldChange describes an audited before-and-after change to one resource field.
//
// Before and After intentionally accept structured JSON-compatible values because
// audited fields may be strings, numbers, booleans, identifiers, or small value
// objects. Callers must include only allowlisted, non-sensitive values.
type FieldChange struct {
	// Field is the allowlisted resource field name that changed.
	Field string `json:"field"`
	// Before is the previous field value, when applicable.
	Before any `json:"before,omitempty"`
	// After is the new field value, when applicable.
	After any `json:"after,omitempty"`
}

// Validate validates a field change.
func (c FieldChange) Validate() error {
	if c.Field == "" {
		return errors.New("field is required")
	}

	return nil
}
