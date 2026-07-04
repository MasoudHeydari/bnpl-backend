package config

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	checkoutorigin "github.com/parstis/bnpl-backend/pkg/core/policy/checkoutorigin"
)

func mustCheckoutSessionToken(t *testing.T, raw string) model.CheckoutSessionToken {
	t.Helper()

	token, err := model.ParseCheckoutSessionToken(raw)
	require.NoError(t, err)

	return token
}

func TestPublicOriginPolicyForEnvironment(t *testing.T) {
	t.Parallel()

	hosts := []string{"bnpl.test", "localhost"}

	dev := publicOriginPolicyForEnvironment(model.DevelopmentRuntimeEnvironment, hosts)
	require.Equal(t, checkoutorigin.Development(hosts), dev)

	testEnv := publicOriginPolicyForEnvironment(model.TestRuntimeEnvironment, hosts)
	require.Equal(t, checkoutorigin.Development(hosts), testEnv)

	prodHosts := []string{"checkout.bnpl.example.com"}
	prod := publicOriginPolicyForEnvironment(model.ProductionRuntimeEnvironment, prodHosts)
	require.Equal(t, checkoutorigin.Strict(prodHosts), prod)

	staging := publicOriginPolicyForEnvironment(model.StagingRuntimeEnvironment, prodHosts)
	require.Equal(t, checkoutorigin.Strict(prodHosts), staging)

	unknown := publicOriginPolicyForEnvironment(model.RuntimeEnvironment("whatever"), prodHosts)
	require.Equal(t, checkoutorigin.Strict(prodHosts), unknown)
}

func TestCheckoutSession_ResolvePublicOrigin_ValidDevelopment(t *testing.T) {
	t.Parallel()

	cfg := CheckoutSession{
		PublicOrigin: "https://bnpl.test",
		PublicOriginPolicy: PublicOriginPolicyConfig{
			AllowedHosts: []string{"bnpl.test"},
		},
		TTL:             30 * time.Minute,
		ExpiryBatchSize: 100,
	}

	origin, builder, err := cfg.resolvePublicOrigin(model.DevelopmentRuntimeEnvironment)
	require.NoError(t, err)
	require.Equal(t, "https://bnpl.test", origin.String())

	token := mustCheckoutSessionToken(t, "cs_"+strings.Repeat("1", 64))
	require.Equal(t, "https://bnpl.test/checkout/"+token.String(), builder.Checkout(token))
}

func TestCheckoutSession_ResolvePublicOrigin_InvalidProductionScheme(t *testing.T) {
	t.Parallel()

	cfg := CheckoutSession{
		PublicOrigin: "http://checkout.bnpl.example.com",
		PublicOriginPolicy: PublicOriginPolicyConfig{
			AllowedHosts: []string{"checkout.bnpl.example.com"},
		},
	}

	_, _, err := cfg.resolvePublicOrigin(model.ProductionRuntimeEnvironment)
	require.Error(t, err)
	require.ErrorIs(t, err, checkoutorigin.ErrInsecureScheme)
}

func TestCheckoutSession_ResolvePublicOrigin_InvalidProductionHost(t *testing.T) {
	t.Parallel()

	cfg := CheckoutSession{
		PublicOrigin: "https://wrong.example.com",
		PublicOriginPolicy: PublicOriginPolicyConfig{
			AllowedHosts: []string{"checkout.bnpl.example.com"},
		},
	}

	_, _, err := cfg.resolvePublicOrigin(model.ProductionRuntimeEnvironment)
	require.Error(t, err)
	require.ErrorIs(t, err, checkoutorigin.ErrHostNotAllowed)
}

func TestCheckoutSession_ResolvePublicOrigin_PathRejected(t *testing.T) {
	t.Parallel()

	cfg := CheckoutSession{
		PublicOrigin: "https://checkout.bnpl.example.com/checkout",
		PublicOriginPolicy: PublicOriginPolicyConfig{
			AllowedHosts: []string{"checkout.bnpl.example.com"},
		},
	}

	_, _, err := cfg.resolvePublicOrigin(model.ProductionRuntimeEnvironment)
	require.Error(t, err)
	require.ErrorIs(t, err, checkoutorigin.ErrOriginInvalid)
}

func TestUsecases_Validate_PublicOriginStartupFailures(t *testing.T) {
	t.Parallel()

	base := validConfig()
	require.NoError(t, base.Usecases.validate("development"))

	t.Run("valid production origin", func(t *testing.T) {
		t.Parallel()

		cfg := validConfig()
		cfg.Usecases.CheckoutSession.PublicOrigin = "https://checkout.bnpl.example.com"
		cfg.Usecases.CheckoutSession.PublicOriginPolicy.AllowedHosts = []string{"checkout.bnpl.example.com"}
		require.NoError(t, cfg.Usecases.validate("production"))
	})

	t.Run("http scheme", func(t *testing.T) {
		t.Parallel()

		cfg := validConfig()
		cfg.Usecases.CheckoutSession.PublicOrigin = "http://checkout.bnpl.example.com"
		cfg.Usecases.CheckoutSession.PublicOriginPolicy.AllowedHosts = []string{"checkout.bnpl.example.com"}

		err := cfg.Usecases.validate("production")
		require.Error(t, err)
		require.ErrorContains(t, err, "public_origin")
		require.ErrorIs(t, err, checkoutorigin.ErrInsecureScheme)
	})

	t.Run("wrong host", func(t *testing.T) {
		t.Parallel()

		cfg := validConfig()
		cfg.Usecases.CheckoutSession.PublicOrigin = "https://wrong.example.com"
		cfg.Usecases.CheckoutSession.PublicOriginPolicy.AllowedHosts = []string{"checkout.bnpl.example.com"}

		err := cfg.Usecases.validate("production")
		require.Error(t, err)
		require.ErrorContains(t, err, "public_origin")
		require.ErrorIs(t, err, checkoutorigin.ErrHostNotAllowed)
	})

	t.Run("path in origin", func(t *testing.T) {
		t.Parallel()

		cfg := validConfig()
		cfg.Usecases.CheckoutSession.PublicOrigin = "https://checkout.bnpl.example.com/checkout"
		cfg.Usecases.CheckoutSession.PublicOriginPolicy.AllowedHosts = []string{"checkout.bnpl.example.com"}

		err := cfg.Usecases.validate("production")
		require.Error(t, err)
		require.ErrorContains(t, err, "public_origin")
		require.ErrorIs(t, err, checkoutorigin.ErrOriginInvalid)
	})

	t.Run("empty query delimiter", func(t *testing.T) {
		t.Parallel()

		cfg := validConfig()
		cfg.Usecases.CheckoutSession.PublicOrigin = "https://checkout.bnpl.example.com?"
		cfg.Usecases.CheckoutSession.PublicOriginPolicy.AllowedHosts = []string{"checkout.bnpl.example.com"}

		err := cfg.Usecases.validate("production")
		require.Error(t, err)
		require.ErrorIs(t, err, checkoutorigin.ErrOriginInvalid)
	})

	t.Run("empty fragment delimiter", func(t *testing.T) {
		t.Parallel()

		cfg := validConfig()
		cfg.Usecases.CheckoutSession.PublicOrigin = "https://checkout.bnpl.example.com#"
		cfg.Usecases.CheckoutSession.PublicOriginPolicy.AllowedHosts = []string{"checkout.bnpl.example.com"}

		err := cfg.Usecases.validate("production")
		require.Error(t, err)
		require.ErrorIs(t, err, checkoutorigin.ErrOriginInvalid)
	})

	t.Run("trailing-dot hostname", func(t *testing.T) {
		t.Parallel()

		cfg := validConfig()
		cfg.Usecases.CheckoutSession.PublicOrigin = "https://checkout.bnpl.example.com."
		cfg.Usecases.CheckoutSession.PublicOriginPolicy.AllowedHosts = []string{"checkout.bnpl.example.com"}

		err := cfg.Usecases.validate("production")
		require.Error(t, err)
		require.ErrorIs(t, err, checkoutorigin.ErrOriginInvalid)
	})

	t.Run("trailing slash", func(t *testing.T) {
		t.Parallel()

		cfg := validConfig()
		cfg.Usecases.CheckoutSession.PublicOrigin = "https://checkout.bnpl.example.com/"
		cfg.Usecases.CheckoutSession.PublicOriginPolicy.AllowedHosts = []string{"checkout.bnpl.example.com"}

		err := cfg.Usecases.validate("production")
		require.Error(t, err)
		require.ErrorIs(t, err, checkoutorigin.ErrOriginInvalid)
	})
}
