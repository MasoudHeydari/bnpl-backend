package audit

import (
	"errors"
	"fmt"
)

// ActorType identifies the kind of principal responsible for an audited action.
type ActorType string

// Known actor types for audit events.
const (
	CustomerActorType ActorType = "customer"
	MerchantActorType ActorType = "merchant"
	OperatorActorType ActorType = "operator"
	ServiceActorType  ActorType = "service"
	SystemActorType   ActorType = "system"
)

// IsValid reports whether the actor type is known by the audit contract.
func (t ActorType) IsValid() bool {
	switch t {
	case CustomerActorType,
		MerchantActorType,
		OperatorActorType,
		ServiceActorType,
		SystemActorType:
		return true

	default:
		return false
	}
}

// Actor identifies the principal responsible for an audited action.
//
// ID is the stable identifier of a human or business principal. Service identifies
// the originating service for service and system actors. At least one of ID or
// Service must be present so the action remains attributable.
type Actor struct {
	// Type identifies the kind of principal responsible for the action.
	Type ActorType `json:"type"`
	// ID is the stable identifier of a human or business principal, when applicable.
	ID *string `json:"id,omitempty"`
	// Service identifies the originating service for service and system actors.
	Service *string `json:"service,omitempty"`
}

// Validate validates actor attribution fields.
func (a Actor) Validate() error {
	const op = "audit.Actor.Validate"

	if !a.Type.IsValid() {
		return fmt.Errorf("%s: unsupported type %q", op, a.Type)
	}

	if (a.ID == nil || *a.ID == "") && (a.Service == nil || *a.Service == "") {
		return errors.New("id or service is required")
	}

	return nil
}
