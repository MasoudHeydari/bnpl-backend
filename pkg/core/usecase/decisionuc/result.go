package decisionuc

// CreateResult identifies how a checkout decision create completed.
type CreateResult string

const (
	// CreateResultCreated indicates a new checkout decision and allocation were persisted.
	CreateResultCreated CreateResult = "created"

	// CreateResultAlreadyExists indicates an idempotent replay returned the existing decision.
	CreateResultAlreadyExists CreateResult = "already_exists"
)
