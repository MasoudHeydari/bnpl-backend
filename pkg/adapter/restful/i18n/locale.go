package i18n

import (
	"context"
	"errors"
	"strings"
)

// Locale identifies a supported response language.
type Locale string

const (
	// LocaleENUS is English (United States).
	LocaleENUS Locale = "en-US"
	// LocaleFAIR is Persian (Iran).
	LocaleFAIR Locale = "fa-IR"
)

// DefaultLocale is used when the request does not select a supported language.
const DefaultLocale = LocaleENUS

// Params contains optional template data for a localized message.
type Params map[string]any

// Localizer is the provider boundary for translating semantic message keys.
//
// Implementations are responsible for rendering params and for returning an
// error when key cannot be resolved. The REST serializer owns the final safe
// fallback behavior, so providers never need to expose internal diagnostics to
// clients.
type Localizer interface {
	Localize(locale Locale, key string, params Params) (string, error)
}

var (
	// ErrInvalidLocale indicates an unsupported locale value.
	ErrInvalidLocale = errors.New("invalid locale")
	// ErrMissingRequestContext indicates that localization middleware did not
	// attach request localization state.
	ErrMissingRequestContext = errors.New("i18n request context is missing")
)

// ParseLocale normalizes a locale token into a supported BCP 47 Locale.
//
// The REST API currently supports only en-US and fa-IR. Matching is
// case-insensitive, while the returned Locale always uses canonical casing.
func ParseLocale(value string) (Locale, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ErrInvalidLocale
	}

	switch {
	case strings.EqualFold(value, string(LocaleENUS)):
		return LocaleENUS, nil

	case strings.EqualFold(value, string(LocaleFAIR)):
		return LocaleFAIR, nil

	default:
		return "", ErrInvalidLocale
	}
}

// String returns the canonical BCP 47 locale token.
func (l Locale) String() string {
	return string(l)
}

// RequestState contains request-scoped localization dependencies.
type RequestState struct {
	Locale    Locale
	Localizer Localizer
}

type requestStateKey struct{}

// WithRequestState attaches locale and localizer to ctx.
func WithRequestState(
	ctx context.Context,
	locale Locale,
	localizer Localizer,
) context.Context {
	return context.WithValue(
		ctx, requestStateKey{},
		RequestState{
			Locale:    locale,
			Localizer: localizer,
		},
	)
}

// RequestStateFromContext retrieves request-scoped localization state.
func RequestStateFromContext(ctx context.Context) (RequestState, bool) {
	state, ok := ctx.Value(requestStateKey{}).(RequestState)
	return state, ok && state.Locale != "" && state.Localizer != nil
}

// Localize resolves key using the request-scoped locale and provider.
func Localize(ctx context.Context, key string, params Params) (string, error) {
	state, ok := RequestStateFromContext(ctx)
	if !ok {
		return "", ErrMissingRequestContext
	}

	return state.Localizer.Localize(state.Locale, key, params)
}
