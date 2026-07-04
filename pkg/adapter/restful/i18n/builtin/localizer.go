package builtin

import (
	"bytes"
	"embed"
	"fmt"
	"text/template"

	"gopkg.in/yaml.v3"

	resti18n "github.com/parstis/bnpl-backend/pkg/adapter/restful/i18n"
)

//go:embed messages/*.yaml
var catalogFS embed.FS

// Localizer renders messages from immutable embedded YAML catalogs.
type Localizer struct {
	defaultLocale resti18n.Locale
	catalogs      map[resti18n.Locale]map[string]string
}

// New loads and validates all embedded catalogs.
//
// Startup fails when en-US/fa-IR catalogs do not have identical key sets. This
// keeps missing translations from becoming a production-only failure.
func New(defaultLocale resti18n.Locale) (*Localizer, error) {
	const op = "builtin.New"

	if _, err := resti18n.ParseLocale(defaultLocale.String()); err != nil {
		return nil, fmt.Errorf("%s: invalid default locale %q: %w", op, defaultLocale, err)
	}

	catalogs := make(map[resti18n.Locale]map[string]string, 2)
	for _, locale := range []resti18n.Locale{resti18n.LocaleENUS, resti18n.LocaleFAIR} {
		catalog, err := loadCatalog(locale)
		if err != nil {
			return nil, err
		}

		catalogs[locale] = catalog
	}

	if err := validateParity(catalogs[resti18n.LocaleENUS], catalogs[resti18n.LocaleFAIR]); err != nil {
		return nil, err
	}

	return &Localizer{defaultLocale: defaultLocale, catalogs: catalogs}, nil
}

// Localize resolves and renders a semantic key.
func (l *Localizer) Localize(locale resti18n.Locale, key string, params resti18n.Params) (string, error) {
	const op = "builtin.Localizer.Localize"

	message, ok := l.catalogs[locale][key]
	if !ok && locale != l.defaultLocale {
		message, ok = l.catalogs[l.defaultLocale][key]
	}

	if !ok {
		return "", fmt.Errorf("%s: translation key %q not found", op, key)
	}

	if len(params) == 0 {
		return message, nil
	}

	tmpl, err := template.New(key).Option("missingkey=error").Parse(message)
	if err != nil {
		return "", fmt.Errorf("%s: parse translation %q: %w", op, key, err)
	}

	var out bytes.Buffer
	if err := tmpl.Execute(&out, params); err != nil {
		return "", fmt.Errorf("%s: render translation %q: %w", op, key, err)
	}

	return out.String(), nil
}

func loadCatalog(locale resti18n.Locale) (map[string]string, error) {
	const op = "builtin.loadCatalog"

	path := fmt.Sprintf("messages/%s.yaml", locale)
	data, err := catalogFS.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%s: read i18n catalog %s: %w", op, path, err)
	}

	catalog := make(map[string]string)
	if err := yaml.Unmarshal(data, &catalog); err != nil {
		return nil, fmt.Errorf("%s: decode i18n catalog %s: %w", op, path, err)
	}

	if len(catalog) == 0 {
		return nil, fmt.Errorf("%s: i18n catalog %s is empty", op, path)
	}

	return catalog, nil
}

func validateParity(enUS, faIR map[string]string) error {
	const op = "builtin.validateParity"

	for key := range enUS {
		if _, ok := faIR[key]; !ok {
			return fmt.Errorf("%s: i18n key %q exists in en-US but not fa-IR", op, key)
		}
	}

	for key := range faIR {
		if _, ok := enUS[key]; !ok {
			return fmt.Errorf("%s: i18n key %q exists in fa-IR but not en-US", op, key)
		}
	}

	return nil
}
