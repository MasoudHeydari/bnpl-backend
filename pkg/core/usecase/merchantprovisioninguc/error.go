package merchantprovisioninguc

import "errors"

// ErrConflict marks an immutable event or command identity conflict.
var ErrConflict = errors.New("merchant participation identity conflict")

// ErrInvalidReference marks a conclusive invalid participation reference.
var ErrInvalidReference = errors.New("invalid merchant participation reference")
