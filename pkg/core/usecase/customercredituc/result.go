package customercredituc

// AssignInitialLimitResult identifies how an initial credit limit assignment completed.
type AssignInitialLimitResult string

const (
	// AssignInitialLimitResultPending indicates a durable request awaiting evaluation.
	AssignInitialLimitResultPending AssignInitialLimitResult = "pending"

	// AssignInitialLimitResultCreated indicates a new customer credit limit was persisted.
	AssignInitialLimitResultCreated AssignInitialLimitResult = "created"

	// AssignInitialLimitResultAlreadyAssigned indicates an active credit limit already exists.
	AssignInitialLimitResultAlreadyAssigned AssignInitialLimitResult = "already_assigned"

	// AssignInitialLimitResultDenied indicates a denied credit limit decision was created or replayed.
	AssignInitialLimitResultDenied AssignInitialLimitResult = "denied"
)
