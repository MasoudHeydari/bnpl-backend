package contractuc

// CreateContractResult identifies how generic checkout contract creation completed.
type CreateContractResult string

const (
	// CreateContractResultCreated indicates a new contract was created.
	CreateContractResultCreated CreateContractResult = "created"

	// CreateContractResultAlreadyExists indicates a contract already exists for the decision.
	CreateContractResultAlreadyExists CreateContractResult = "already_exists"
)

// ActivateResult identifies how contract activation completed.
type ActivateResult string

const (
	// ActivateResultActivated indicates the contract was activated after its
	// policy-required Financial Core fulfillment completed authoritatively.
	ActivateResultActivated ActivateResult = "activated"

	// ActivateResultAlreadyActive indicates the contract was already active.
	ActivateResultAlreadyActive ActivateResult = "already_active"
)
