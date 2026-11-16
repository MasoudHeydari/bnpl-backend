package log

// Field is a typed structured attribute attached to a log record.
//
// Adapter field constructors produce allowlisted Fields. The concrete Logger
// adapter must still validate Field keys and value kinds before emission.
type Field interface {
	// Key returns the attribute name (for example "session_id").
	Key() string

	// Value returns the attribute value. Prefer scalars and domain ID strings.
	Value() any
}

// attr is a Core-safe Field used by TextAttr and by adapters that already
// validated the key.
type attr struct {
	key   string
	value any
}

// TextAttr returns a Field with an explicit key and value.
//
// Prefer adapter field constructors for business attributes so allowlisting
// and redaction stay centralized. TextAttr is for operational attributes
// that Core packages must emit without importing Adapter.
func TextAttr(key string, value any) Field {
	return attr{key: key, value: value}
}

func (a attr) Key() string {
	return a.key
}

func (a attr) Value() any {
	return a.value
}
