// Package serdser provides request deserialization and response serialization
// helpers for the REST transport layer.
//
// serdser translates between HTTP requests/responses and application-layer
// contracts. Every HTTP endpoint should produce responses through this package
// to ensure a consistent API contract. Application and domain layers must
// remain independent of serdser and communicate failures through cerr.Error
// values.
//
// Localization contract:
//   - Localize only system-generated prose that a client may render verbatim to
//     a human, such as response messages and validation violation messages.
//   - Keep machine-readable protocol values canonical and language-neutral.
//     This includes status, code, result, outcome, reason_codes, pagination
//     type, currency, and health-probe states such as UP.
//   - Business-owned content such as merchant display_name/legal_name is
//     returned as stored. It is not translated by the system message catalog.
//
// Response fields containing localizable prose are explicitly marked with the
// struct tag i18n:"localized" and are guarded by source-level contract tests.
package serdser
