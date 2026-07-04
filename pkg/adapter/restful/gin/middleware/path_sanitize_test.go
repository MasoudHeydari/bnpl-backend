package middleware_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/middleware"
)

func TestSanitizeHTTPPathForLog_RedactsCheckoutPublicToken(t *testing.T) {
	t.Parallel()

	token := "cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	require.Equal(
		t,
		"/api/v1/hosted/checkout-sessions/cs_[REDACTED]/confirm",
		middleware.SanitizeHTTPPathForLog("/api/v1/hosted/checkout-sessions/"+token+"/confirm"),
	)
	require.Equal(
		t,
		"/api/v1/hosted/checkout-sessions/cs_[REDACTED]",
		middleware.SanitizeHTTPPathForLog("/api/v1/hosted/checkout-sessions/"+token),
	)
	require.Equal(
		t,
		"/api/v1/partner/checkout-sessions",
		middleware.SanitizeHTTPPathForLog("/api/v1/partner/checkout-sessions"),
	)
}

func TestSanitizeHTTPRefererForLog_RedactsCheckoutPublicToken(t *testing.T) {
	t.Parallel()

	token := "cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	require.Equal(
		t,
		"https://bnpl.example/api/v1/hosted/checkout-sessions/cs_[REDACTED]/confirm",
		middleware.SanitizeHTTPRefererForLog(
			"https://bnpl.example/api/v1/hosted/checkout-sessions/"+token+"/confirm",
		),
	)
	require.Equal(
		t,
		"https://bnpl.example/api/v1/hosted/checkout-sessions/cs_[REDACTED]",
		middleware.SanitizeHTTPRefererForLog(
			"https://bnpl.example/api/v1/hosted/checkout-sessions/"+token+"?x=1#frag",
		),
	)
	require.NotContains(
		t,
		middleware.SanitizeHTTPRefererForLog(
			"https://bnpl.example/api/v1/hosted/checkout-sessions/"+token,
		),
		token,
	)
}
