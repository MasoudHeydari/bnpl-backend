// Package cerr provides structured application errors.
//
// cerr is the canonical error model used across the application layer. It
// provides stable machine-readable error codes, optional structured metadata,
// and error wrapping via errors.Is/errors.As. Transport adapters are
// responsible for translating cerr.Error values into protocol-specific
// responses.
package cerr
