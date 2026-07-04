// Package phonenumber registers jurisdiction-specific phone canonicalizers.
//
// This adapter lives under the canonicalizer namespace. That namespace groups
// concept-specific adapters; it is not a universal Canonicalizer interface.
//
// domaincapability.Jurisdictions is the runtime authority for which
// jurisdictions may originate new BNPL business. The phone registry consumes
// that capability to ensure every origination-enabled jurisdiction has a
// compiled adapter. Phone Registry is not the business-admission authority.
//
// Core depends only on phonenumber.Canonicalizer; country syntax lives in
// subpackages such as Iran.
package phonenumber
