package saga

import (
	"errors"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

var (
	// ErrDefinitionNotFound is returned when no Saga definition is registered for
	// a requested Saga type and definition version.
	ErrDefinitionNotFound = errors.New("saga definition not found")
)

// DefinitionResolver resolves a Saga definition by its durable type and
// definition version.
//
// Execution infrastructure can depend on this narrow interface without knowing
// which concrete Saga types or versions exist in the application.
type DefinitionResolver interface {
	// Definition resolves a Saga definition by durable type and version.
	// The boolean is false when no matching definition is registered.
	Definition(
		sagaType model.SagaType,
		version model.SagaDefinitionVersion,
	) (Definition, bool)
}

type definitionKey struct {
	sagaType model.SagaType
	version  model.SagaDefinitionVersion
}

// Registry is an immutable in-memory registry of validated Saga definitions.
//
// Multiple versions of the same Saga type may be registered simultaneously so
// in-flight durable instances can continue using the exact definition version
// with which they were created.
type Registry struct {
	definitions map[definitionKey]Definition
}

// NewRegistry validates and registers the supplied definitions.
//
// Duplicate (SagaType, DefinitionVersion) pairs are rejected because resolving
// the same durable identity to different definitions would make persisted Saga
// state ambiguous.
func NewRegistry(definitions ...Definition) (*Registry, error) {
	const op = "saga.NewRegistry"

	registry := &Registry{
		definitions: make(map[definitionKey]Definition, len(definitions)),
	}

	for _, definition := range definitions {
		if err := ValidateDefinition(definition); err != nil {
			return nil, err
		}

		key := definitionKey{
			sagaType: definition.Type(),
			version:  definition.Version(),
		}
		if _, exists := registry.definitions[key]; exists {
			return nil, fmt.Errorf(
				"%s: %w: duplicate saga definition type %q version %d", op, ErrInvalidDefinition,
				key.sagaType,
				key.version,
			)
		}

		registry.definitions[key] = definition
	}

	return registry, nil
}

// Definition returns the registered definition for sagaType and version.
func (r *Registry) Definition(
	sagaType model.SagaType,
	version model.SagaDefinitionVersion,
) (Definition, bool) {
	if r == nil {
		return nil, false
	}

	definition, ok := r.definitions[definitionKey{
		sagaType: sagaType,
		version:  version,
	}]
	return definition, ok
}

// RequireDefinition returns the registered definition for sagaType and version
// or a descriptive error when the durable definition identity is unknown.
func (r *Registry) RequireDefinition(
	sagaType model.SagaType,
	version model.SagaDefinitionVersion,
) (Definition, error) {
	const op = "saga.Registry.RequireDefinition"

	if !sagaType.IsValid() {
		return nil, fmt.Errorf("%s: %w: saga type is required", op, ErrDefinitionNotFound)
	}

	if !version.IsValid() {
		return nil, fmt.Errorf("%s: %w: definition version is required", op, ErrDefinitionNotFound)
	}

	definition, ok := r.Definition(sagaType, version)
	if !ok {
		return nil, fmt.Errorf(
			"%s: %w: type %q version %d", op, ErrDefinitionNotFound,
			sagaType,
			version,
		)
	}

	return definition, nil
}

var _ DefinitionResolver = (*Registry)(nil)
