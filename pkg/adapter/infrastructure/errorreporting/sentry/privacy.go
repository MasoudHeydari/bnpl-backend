package sentry

import (
	"strings"

	sentrygo "github.com/getsentry/sentry-go"
)

const redactedValue = "[Filtered]"

var sensitiveKeyFragments = []string{
	"authorization",
	"cookie",
	"credential",
	"password",
	"passwd",
	"secret",
	"token",
	"api_key",
	"apikey",
	"national_id",
	"nationalid",
	"phone",
	"mobile",
	"email",
	"card_number",
	"cardnumber",
	"card_pan",
	"cvv",
	"pin",
}

// sanitizeEvent is a last privacy boundary before an event leaves the process.
//
// The application does not attach HTTP requests to Sentry scopes, but clearing
// request/user/breadcrumb data here protects against future instrumentation
// accidentally adding request-derived PII without an explicit review.
func sanitizeEvent(event *sentrygo.Event, _ *sentrygo.EventHint) *sentrygo.Event {
	if event == nil {
		return nil
	}

	event.Request = nil
	event.User = sentrygo.User{}
	event.Breadcrumbs = nil

	// Event.Extra was removed in sentry-go v0.49; structured metadata now lives
	// in Contexts (and scope attributes). Scrub those maps instead.
	for name, values := range event.Contexts {
		if sensitiveKey(name) {
			delete(event.Contexts, name)
			continue
		}

		event.Contexts[name] = sanitizeMap(values)
	}

	for key := range event.Tags {
		if sensitiveKey(key) {
			event.Tags[key] = redactedValue
		}
	}

	return event
}

func sanitizeMap(values map[string]any) map[string]any {
	if len(values) == 0 {
		return values
	}

	clean := make(map[string]any, len(values))
	for key, value := range values {
		if sensitiveKey(key) {
			clean[key] = redactedValue
			continue
		}

		clean[key] = sanitizeValue(value)
	}

	return clean
}

func sanitizeValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return sanitizeMap(typed)

	case map[string]string:
		clean := make(map[string]string, len(typed))
		for key, item := range typed {
			if sensitiveKey(key) {
				clean[key] = redactedValue
				continue
			}

			clean[key] = item
		}

		return clean

	case []any:
		clean := make([]any, len(typed))
		for idx, item := range typed {
			clean[idx] = sanitizeValue(item)
		}

		return clean

	default:
		return value
	}
}

func sensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	normalized = strings.ReplaceAll(normalized, "-", "_")

	// Keep the generic PAN field protected without treating every key that
	// merely contains the letters "pan" (for example "span_id") as sensitive.
	if normalized == "pan" {
		return true
	}

	for _, fragment := range sensitiveKeyFragments {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}

	return false
}
