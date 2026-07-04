package sentry

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sentrygo "github.com/getsentry/sentry-go"
)

func TestSanitizeEvent_RemovesRequestUserAndBreadcrumbs(t *testing.T) {
	t.Parallel()

	event := sentrygo.NewEvent()
	event.Request = &sentrygo.Request{URL: "https://example.test/?token=secret"}
	event.User = sentrygo.User{ID: "customer-123", Email: "user@example.test"}
	event.Breadcrumbs = []*sentrygo.Breadcrumb{{Message: "request body"}}

	clean := sanitizeEvent(event, nil)

	require.NotNil(t, clean)
	assert.Nil(t, clean.Request)
	assert.Equal(t, sentrygo.User{}, clean.User)
	assert.Nil(t, clean.Breadcrumbs)
}

func TestSanitizeEvent_RedactsSensitiveContextKeys(t *testing.T) {
	t.Parallel()

	event := sentrygo.NewEvent()
	event.Contexts["request"] = sentrygo.Context{
		"trace_id":      "trace-123",
		"authorization": "Bearer secret",
		"nested": map[string]any{
			"phone": "+989121234567",
			"safe":  "value",
		},
	}
	event.Contexts["app"] = sentrygo.Context{
		"api-key": "secret",
		"safe":    "value",
	}

	clean := sanitizeEvent(event, nil)

	require.NotNil(t, clean)
	assert.Equal(t, "trace-123", clean.Contexts["request"]["trace_id"])
	assert.Equal(t, redactedValue, clean.Contexts["request"]["authorization"])
	assert.Equal(t, redactedValue, clean.Contexts["app"]["api-key"])
	assert.Equal(t, "value", clean.Contexts["app"]["safe"])

	nested, ok := clean.Contexts["request"]["nested"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, redactedValue, nested["phone"])
	assert.Equal(t, "value", nested["safe"])
}

func TestSensitiveKey_DoesNotTreatSpanIDAsCardPAN(t *testing.T) {
	t.Parallel()

	assert.False(t, sensitiveKey("span_id"))
	assert.True(t, sensitiveKey("pan"))
}
