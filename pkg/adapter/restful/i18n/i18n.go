// Package i18n defines the transport-side localization contract used by REST.
//
// It owns locale parsing and request-scoped localizer state. Concrete catalogs
// live in subpackages such as builtin so handlers and serializers stay
// provider-agnostic.
package i18n
