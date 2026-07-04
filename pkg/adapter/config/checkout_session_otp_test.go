package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	identityext "github.com/parstis/bnpl-backend/pkg/adapter/external/identity"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

const testOTPPepper = "test-checkout-otp-pepper-0123456789ab"

func TestCheckoutSession_OTPDefaults(t *testing.T) {
	t.Parallel()

	cfg := CheckoutSession{}

	require.Equal(t, "bnpl.external_checkout_otp.v1", cfg.EffectiveOTPTemplateID())

	cfg = CheckoutSession{OTPTemplateID: " custom.v2 "}

	require.Equal(t, "custom.v2", cfg.EffectiveOTPTemplateID())
}

func TestCheckoutSession_NewOTPHasher(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     CheckoutSession
		wantErr string
	}{
		{
			name: "single key becomes active",
			cfg: CheckoutSession{OTPPepperKeys: KeyEntries{
				{ID: "o1", Secret: model.NewSecret(testOTPPepper)},
			}},
		},
		{
			name: "explicit active key in ring",
			cfg: CheckoutSession{
				OTPActivePepperKeyID: "o2",
				OTPPepperKeys: KeyEntries{
					{ID: "o1", Secret: model.NewSecret(testOTPPepper)},
					{ID: "o2", Secret: model.NewSecret(testOTPPepper + "-2")},
				},
			},
		},
		{
			name:    "missing ring",
			cfg:     CheckoutSession{},
			wantErr: "otp_pepper_keys is required",
		},
		{
			name: "multiple keys require active id",
			cfg: CheckoutSession{OTPPepperKeys: KeyEntries{
				{ID: "o1", Secret: model.NewSecret(testOTPPepper)},
				{ID: "o2", Secret: model.NewSecret(testOTPPepper + "-2")},
			}},
			wantErr: "otp_active_pepper_key_id is required",
		},
		{
			name: "active key not in ring",
			cfg: CheckoutSession{
				OTPActivePepperKeyID: "o9",
				OTPPepperKeys:        KeyEntries{{ID: "o1", Secret: model.NewSecret(testOTPPepper)}},
			},
			wantErr: "not in the key ring",
		},
		{
			name: "duplicate ids",
			cfg: CheckoutSession{OTPPepperKeys: KeyEntries{
				{ID: "o1", Secret: model.NewSecret(testOTPPepper)},
				{ID: "o1", Secret: model.NewSecret(testOTPPepper + "-2")},
			}},
			wantErr: "duplicate",
		},
		{
			name: "short pepper",
			cfg: CheckoutSession{OTPPepperKeys: KeyEntries{
				{ID: "o1", Secret: model.NewSecret("short")},
			}},
			wantErr: "create hasher",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			hasher, err := tt.cfg.NewOTPHasher()
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.NotContains(t, err.Error(), testOTPPepper)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, hasher)
		})
	}
}

func TestCheckoutSession_ValidateOTP(t *testing.T) {
	t.Parallel()

	valid := func() CheckoutSession {
		return CheckoutSession{OTPPepperKeys: KeyEntries{{ID: "o1", Secret: model.NewSecret(testOTPPepper)}}}
	}

	require.NoError(t, valid().validateOTP())
	require.ErrorContains(t, CheckoutSession{}.validateOTP(), "otp pepper")
}

func TestExternalIdentity_Validate(t *testing.T) {
	t.Parallel()

	dev := model.RuntimeEnvironment("development")
	prod := model.RuntimeEnvironment("production")

	tests := []struct {
		name    string
		cfg     ExternalIdentity
		env     model.RuntimeEnvironment
		wantErr string
	}{
		{name: "noop in development", cfg: ExternalIdentity{Provider: string(identityext.NoopProvider)}, env: dev},
		{name: "noop rejected in production", cfg: ExternalIdentity{Provider: string(identityext.NoopProvider)}, env: prod, wantErr: "not allowed in production"},
		{name: "unsupported provider", cfg: ExternalIdentity{Provider: "smtp"}, env: dev, wantErr: "unsupported"},
		{name: "empty provider", cfg: ExternalIdentity{}, env: dev, wantErr: "unsupported"},
		{
			name: "grpc ok",
			cfg:  ExternalIdentity{Provider: string(identityext.GRPCProvider), GRPC: ExternalIdentityGRPC{Address: "id:50053", Timeout: time.Second, Insecure: true}},
			env:  dev,
		},
		{
			name:    "grpc requires address",
			cfg:     ExternalIdentity{Provider: string(identityext.GRPCProvider), GRPC: ExternalIdentityGRPC{Timeout: time.Second}},
			env:     dev,
			wantErr: "grpc.address",
		},
		{
			name:    "grpc requires timeout",
			cfg:     ExternalIdentity{Provider: string(identityext.GRPCProvider), GRPC: ExternalIdentityGRPC{Address: "id:50053"}},
			env:     dev,
			wantErr: "grpc.timeout",
		},
		{
			name:    "grpc insecure rejected in production",
			cfg:     ExternalIdentity{Provider: string(identityext.GRPCProvider), GRPC: ExternalIdentityGRPC{Address: "id:50053", Timeout: time.Second, Insecure: true}},
			env:     prod,
			wantErr: "insecure",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.cfg.validate(tt.env)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
		})
	}
}
