package config

import (
	"fmt"
	"net/url"
	"strings"
)

const maxPaymentReturnURLLength = 2048

// validateHTTPSURL validates that raw is an absolute HTTPS URL without embedded credentials.
func validateHTTPSURL(fieldPath, raw string) error {
	const op = "config.validateHTTPSURL"

	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("%s: invalid %s: value is required", op, fieldPath)
	}

	if len(raw) > maxPaymentReturnURLLength {
		return fmt.Errorf("%s: invalid %s: value exceeds maximum length %d", op, fieldPath, maxPaymentReturnURLLength)
	}

	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("%s: invalid %s: URL is malformed", op, fieldPath)
	}

	if parsed.Scheme != "https" {
		return fmt.Errorf("%s: invalid %s: URL must use HTTPS", op, fieldPath)
	}

	if parsed.User != nil {
		return fmt.Errorf("%s: invalid %s: URL must not contain user information", op, fieldPath)
	}

	return nil
}
