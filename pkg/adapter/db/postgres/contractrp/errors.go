package contractrp

import (
	"errors"
)

var (
	// ErrContractNotFound is returned when a contract row does not exist.
	ErrContractNotFound = errors.New("contract not found")
)
