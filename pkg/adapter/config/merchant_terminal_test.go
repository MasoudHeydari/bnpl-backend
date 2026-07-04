package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	fingerprinthmac "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/fingerprint/hmac"
	"github.com/parstis/bnpl-backend/pkg/core/fingerprint"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func validMerchantTerminal() MerchantTerminal {
	return MerchantTerminal{
		APIKeyActivePepperKeyID: "v2",
		APIKeyPepperKeys: KeyEntries{
			{ID: "v1", Secret: model.NewSecret("test-merchant-terminal-api-key-pepper-v1")},
			{ID: "v2", Secret: model.NewSecret("test-merchant-terminal-api-key-pepper-v2")},
		},
		Rotation: MerchantTerminalRotation{
			DefaultGracePeriod: 24 * time.Hour,
			MaxGracePeriod:     7 * 24 * time.Hour,
		},
		IssuanceDeliveryTTL: 24 * time.Hour,
		IssuanceEncryptionKeys: KeyEntries{
			{ID: "m1", Secret: model.NewSecret("dGVzdC1tZXJjaGFudC1hdXRoLWlzc3VhbmNlLWtleSE=")},
		},
	}
}

func TestMerchantTerminal_Validate(t *testing.T) {
	t.Parallel()

	require.NoError(t, validMerchantTerminal().validate())

	tests := map[string]struct {
		mutate  func(*MerchantTerminal)
		message string
	}{
		"short pepper": {
			mutate: func(m *MerchantTerminal) {
				m.APIKeyPepperKeys = KeyEntries{
					{ID: "v2", Secret: model.NewSecret("short")},
				}
			},
			message: "api key pepper settings",
		},
		"missing active pepper key": {
			mutate: func(m *MerchantTerminal) {
				m.APIKeyActivePepperKeyID = "missing"
			},
			message: "api key pepper settings",
		},
		"empty pepper ring and legacy": {
			mutate: func(m *MerchantTerminal) {
				m.APIKeyPepperKeys = nil
				m.APIKeyPepper = model.Secret{}
				m.APIKeyActivePepperKeyID = ""
			},
			message: "api key pepper settings",
		},
		"default grace not positive": {
			mutate:  func(m *MerchantTerminal) { m.Rotation.DefaultGracePeriod = 0 },
			message: "usecases.merchant_terminal.rotation.default_grace_period",
		},
		"max grace not positive": {
			mutate:  func(m *MerchantTerminal) { m.Rotation.MaxGracePeriod = 0 },
			message: "usecases.merchant_terminal.rotation.max_grace_period",
		},
		"default grace above max": {
			mutate: func(m *MerchantTerminal) {
				m.Rotation.DefaultGracePeriod = 2 * time.Hour
				m.Rotation.MaxGracePeriod = time.Hour
			},
			message: "must not exceed max_grace_period",
		},
		"delivery ttl not positive": {
			mutate:  func(m *MerchantTerminal) { m.IssuanceDeliveryTTL = 0 },
			message: "usecases.merchant_terminal.issuance_delivery_ttl",
		},
		"missing encryption keys": {
			mutate:  func(m *MerchantTerminal) { m.IssuanceEncryptionKeys = nil },
			message: "issuance encryption",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cfg := validMerchantTerminal()
			tc.mutate(&cfg)

			err := cfg.validate()
			require.Error(t, err)
			require.ErrorContains(t, err, tc.message)
			require.NotContains(t, err.Error(), "test-merchant-terminal-api-key-pepper")
		})
	}
}

func TestMerchantTerminal_LegacySinglePepper(t *testing.T) {
	t.Parallel()

	cfg := MerchantTerminal{
		APIKeyPepper: model.NewSecret("test-merchant-terminal-api-key-pepper-32b"),
		Rotation: MerchantTerminalRotation{
			DefaultGracePeriod: 24 * time.Hour,
			MaxGracePeriod:     7 * 24 * time.Hour,
		},
		IssuanceDeliveryTTL: 24 * time.Hour,
		IssuanceEncryptionKeys: KeyEntries{
			{ID: "m1", Secret: model.NewSecret("dGVzdC1tZXJjaGFudC1hdXRoLWlzc3VhbmNlLWtleSE=")},
		},
	}

	require.NoError(t, cfg.validate())

	hasher, err := cfg.NewHasher()
	require.NoError(t, err)

	value, err := hasher.Hash("bnpl_secret")
	require.NoError(t, err)
	require.Equal(t, fingerprint.LegacyUnversionedPepperKeyID, value.KeyID)
	require.True(t, hasher.Equal("bnpl_secret", value.String()))
}

func TestMerchantTerminal_PepperKeyRingVersions(t *testing.T) {
	t.Parallel()

	cfg := validMerchantTerminal()
	hasher, err := cfg.NewHasher()
	require.NoError(t, err)

	raw := "bnpl_ring_secret"
	active, err := hasher.Hash(raw)
	require.NoError(t, err)
	require.Equal(t, "v2", active.KeyID)
	require.True(t, hasher.Equal(raw, active.String()))

	v1Hasher, err := fingerprinthmac.NewKeyRing("v1", map[string][]byte{
		"v1": []byte("test-merchant-terminal-api-key-pepper-v1"),
		"v2": []byte("test-merchant-terminal-api-key-pepper-v2"),
	})
	require.NoError(t, err)

	legacy, err := v1Hasher.Hash(raw)
	require.NoError(t, err)
	require.Equal(t, "v1", legacy.KeyID)
	require.True(t, hasher.Equal(raw, legacy.String()), "v2-active ring must still verify v1 digests")

	missingRing, err := fingerprinthmac.NewKeyRing("v2", map[string][]byte{
		"v2": []byte("test-merchant-terminal-api-key-pepper-v2"),
	})
	require.NoError(t, err)
	require.False(t, missingRing.Equal(raw, legacy.String()), "missing referenced pepper must fail closed")
}

func TestMerchantTerminal_UseCaseConfig(t *testing.T) {
	t.Parallel()

	cfg := validMerchantTerminal().UseCaseConfig()

	require.Equal(t, 24*time.Hour, cfg.DefaultGrace)
	require.Equal(t, 7*24*time.Hour, cfg.MaxGrace)
	require.Equal(t, 24*time.Hour, cfg.IssuanceDeliveryTTL)
	require.NoError(t, cfg.Validate())
}
