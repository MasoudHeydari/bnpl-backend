package field_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/field"
	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestSanitize_AllowlistedScalars(t *testing.T) {
	t.Parallel()

	key, value, ok := field.Sanitize(field.Outcome("approved"))
	require.True(t, ok)
	require.Equal(t, field.KeyOutcome, key)
	require.Equal(t, "approved", value)

	key, value, ok = field.Sanitize(field.Duration(1500 * time.Millisecond))
	require.True(t, ok)
	require.Equal(t, field.KeyDurationMS, key)
	require.Equal(t, int64(1500), value)
}

func TestSanitize_RejectsUnknownKey(t *testing.T) {
	t.Parallel()

	_, _, ok := field.Sanitize(log.TextAttr("password", "secret"))
	require.False(t, ok)
}

func TestSanitize_RejectsUnsafeValueKinds(t *testing.T) {
	t.Parallel()

	_, _, ok := field.Sanitize(log.TextAttr(field.KeyError, map[string]any{"raw": true}))
	require.False(t, ok)
}

func TestSanitize_RedactsRecipientPhone(t *testing.T) {
	t.Parallel()

	key, value, ok := field.Sanitize(field.RecipientPhone("+989121234567"))
	require.True(t, ok)
	require.Equal(t, field.KeyRecipientPhone, key)
	require.Equal(t, "+***4567", value)

	_, value, ok = field.Sanitize(log.TextAttr(field.KeyRecipientPhone, "not-a-phone"))
	require.True(t, ok)
	require.Equal(t, "[redacted]", value)
}

func TestSessionID_UsesDomainString(t *testing.T) {
	t.Parallel()

	var id model.CheckoutSessionID
	key, value, ok := field.Sanitize(field.SessionID(id))
	require.True(t, ok)
	require.Equal(t, field.KeySessionID, key)
	require.Equal(t, id.String(), value)
}

func TestRedactPhone_EmptyAndShort(t *testing.T) {
	t.Parallel()

	require.Equal(t, "[redacted]", field.RedactPhone(""))
	require.Equal(t, "[redacted]", field.RedactPhone("1234"))
}
