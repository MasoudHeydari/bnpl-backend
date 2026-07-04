package i18n

import (
	"sort"
	"strconv"
	"strings"
)

// ResolveLocale resolves the response locale using standard HTTP language
// preference first, then the legacy lang header, and finally defaultLocale.
func ResolveLocale(acceptLanguage, lang string, defaultLocale Locale) Locale {
	if locale, ok := resolveAcceptLanguage(acceptLanguage); ok {
		return locale
	}

	if locale, err := ParseLocale(lang); err == nil {
		return locale
	}

	if locale, err := ParseLocale(defaultLocale.String()); err == nil {
		return locale
	}

	return DefaultLocale
}

type languagePreference struct {
	locale Locale
	q      float64
	order  int
}

func resolveAcceptLanguage(value string) (Locale, bool) {
	preferences := make([]languagePreference, 0)

	for order, raw := range strings.Split(value, ",") {
		parts := strings.Split(strings.TrimSpace(raw), ";")
		if len(parts) == 0 {
			continue
		}

		locale, err := ParseLocale(parts[0])
		if err != nil {
			continue
		}

		q := 1.0
		for _, parameter := range parts[1:] {
			parameter = strings.TrimSpace(parameter)
			if !strings.HasPrefix(parameter, "q=") {
				continue
			}

			parsed, parseErr := strconv.ParseFloat(strings.TrimPrefix(parameter, "q="), 64)
			if parseErr == nil {
				q = parsed
			}
		}

		if q <= 0 {
			continue
		}

		preferences = append(
			preferences,
			languagePreference{
				locale: locale,
				q:      q,
				order:  order,
			},
		)
	}

	if len(preferences) == 0 {
		return "", false
	}

	sort.SliceStable(
		preferences,
		func(i, j int) bool {
			if preferences[i].q == preferences[j].q {
				return preferences[i].order < preferences[j].order
			}

			return preferences[i].q > preferences[j].q
		},
	)

	return preferences[0].locale, true
}
