// Package serdser provides request deserialization and response serialization
// helpers for the REST transport layer.
//
// serdser translates between HTTP requests/responses and application-layer
// contracts. Every HTTP endpoint should produce responses through this package
// to ensure a consistent API contract. Application and domain layers must
// remain independent of serdser and communicate failures through cerr.Error
// values.
package serdser
