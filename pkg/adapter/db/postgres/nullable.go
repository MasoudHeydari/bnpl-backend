package postgres

// Nullable returns a query argument that encodes as SQL NULL when v is nil.
//
// In Go, a nil pointer stored in an interface is a typed nil, not an untyped
// nil. Those are not the same value:
//
//	var id *uuid.UUID // nil pointer
//	var a any = id    // typed nil:  a != nil, dynamic type is *uuid.UUID
//	var b any = Nullable(id) // untyped nil: b == nil
//
// Drivers and log formatters often treat a non-nil interface that implements
// database/sql/driver.Valuer (or fmt.Stringer) by calling Value/String on it.
// Many such methods use a value receiver on T, so calling them through a nil
// *T panics ("value method T.Value called using nil *T pointer"). Passing the
// typed nil also bypasses the simple `arg == nil` / SQL NULL short-circuit.
//
// Nullable converts a nil *T into an untyped nil interface value so callers can
// safely bind optional columns. A non-nil pointer is returned unchanged.
func Nullable[T any](v *T) any {
	if v == nil {
		return nil
	}

	return v
}
