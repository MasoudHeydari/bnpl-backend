package audit

import "errors"

// Resource identifies the domain or operational object affected by an audited action.
//
// Type is a stable machine-readable resource name such as "contract" or
// "customer_credit_limit". ID is the resource identifier used for correlation,
// partitioning, and forensic lookup.
type Resource struct {
	// Type is the stable machine-readable resource name.
	Type string `json:"type"`
	// ID is the resource identifier used for correlation and forensic lookup.
	ID string `json:"id"`
}

// Validate validates the audited resource identity.
func (r Resource) Validate() error {
	if r.Type == "" {
		return errors.New("type is required")
	}

	if r.ID == "" {
		return errors.New("id is required")
	}

	return nil
}
