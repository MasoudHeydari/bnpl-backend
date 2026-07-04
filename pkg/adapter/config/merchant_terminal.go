package config

import (
	"fmt"
	"time"

	fingerprinthmac "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/fingerprint/hmac"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/secretbox"
	"github.com/parstis/bnpl-backend/pkg/core/clock"
	"github.com/parstis/bnpl-backend/pkg/core/fingerprint"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	coresecretbox "github.com/parstis/bnpl-backend/pkg/core/secretbox"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/merchantterminaluc"
)

// MerchantTerminal configures external partner terminal authentication and
// Merchant Admin Panel credential management (provision/rotate/retire).
type MerchantTerminal struct {
	// APIKeyPepper is the legacy single pepper. When APIKeyPepperKeys is empty,
	// it is registered as fingerprint.LegacyUnversionedPepperKeyID ("v1") so
	// existing deployments keep working. Prefer APIKeyPepperKeys.
	APIKeyPepper model.Secret `yaml:"api_key_pepper"`

	// APIKeyActivePepperKeyID selects the pepper used to hash newly issued API
	// keys. It must exist in APIKeyPepperKeys (or the legacy single pepper).
	APIKeyActivePepperKeyID string `yaml:"api_key_active_pepper_key_id"`

	// APIKeyPepperKeys is the versioned pepper key ring. Verification uses the
	// pepper key id embedded in each stored verifier; missing keys fail closed.
	APIKeyPepperKeys KeyEntries `yaml:"api_key_pepper_keys"`

	// Rotation tunes credential rotation grace periods.
	Rotation MerchantTerminalRotation `yaml:"rotation"`

	// IssuanceDeliveryTTL is how long an issued API key can be replayed to an
	// idempotent retry of provisioning or rotation.
	IssuanceDeliveryTTL time.Duration `yaml:"issuance_delivery_ttl"`

	// IssuanceEncryptionActiveKeyID selects the AES-GCM key that encrypts newly
	// issued API keys held for idempotent replay. It defaults to the only key
	// when exactly one is configured.
	IssuanceEncryptionActiveKeyID string `yaml:"issuance_encryption_active_key_id"`

	// IssuanceEncryptionKeys are all AES-256 keys (base64, 32 bytes) accepted for
	// decrypting held API keys, including the active one. These keys are
	// deliberately separate from the webhook secret keys.
	IssuanceEncryptionKeys KeyEntries `yaml:"issuance_encryption_keys"`
}

// MerchantTerminalRotation tunes credential rotation.
type MerchantTerminalRotation struct {
	// DefaultGracePeriod is how long a rotated credential stays accepted when
	// the caller does not choose a grace period.
	DefaultGracePeriod time.Duration `yaml:"default_grace_period"`

	// MaxGracePeriod is the longest grace period a caller may request.
	MaxGracePeriod time.Duration `yaml:"max_grace_period"`
}

// NewHasher constructs the versioned peppered HMAC fingerprint hasher used for
// partner API key digests.
func (m MerchantTerminal) NewHasher() (*fingerprinthmac.Hasher, error) {
	const op = "config.MerchantTerminal.NewHasher"

	activeID, peppers, err := m.resolvePepperRing()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	hasher, err := fingerprinthmac.NewKeyRing(activeID, peppers)
	if err != nil {
		return nil, fmt.Errorf("%s: create hasher: %w", op, err)
	}

	return hasher, nil
}

// resolvePepperRing builds the active pepper id and pepper map. Prefer the
// explicit key ring; fall back to the legacy single api_key_pepper as "v1".
func (m MerchantTerminal) resolvePepperRing() (string, map[string][]byte, error) {
	const op = "config.MerchantTerminal.resolvePepperRing"

	peppers := make(map[string][]byte, len(m.APIKeyPepperKeys)+1)

	for _, entry := range m.APIKeyPepperKeys {
		if err := fingerprint.ValidatePepperKeyID(entry.ID); err != nil {
			return "", nil, fmt.Errorf("%s: api_key_pepper_keys entry: %w", op, err)
		}

		if _, exists := peppers[entry.ID]; exists {
			return "", nil, fmt.Errorf("%s: duplicate api_key_pepper_keys id %q", op, entry.ID)
		}

		if entry.Secret.Reveal() == "" {
			return "", nil, fmt.Errorf("%s: api_key_pepper_keys entry %q secret is required", op, entry.ID)
		}

		peppers[entry.ID] = []byte(entry.Secret.Reveal())
	}

	if len(peppers) == 0 {
		if m.APIKeyPepper.Reveal() == "" {
			return "", nil, fmt.Errorf("%s: api_key_pepper_keys or api_key_pepper is required", op)
		}

		peppers[fingerprint.LegacyUnversionedPepperKeyID] = []byte(m.APIKeyPepper.Reveal())
	}

	activeID := m.APIKeyActivePepperKeyID
	if activeID == "" {
		if len(peppers) == 1 {
			for id := range peppers {
				activeID = id
			}
		} else if _, ok := peppers[fingerprint.LegacyUnversionedPepperKeyID]; ok {
			activeID = fingerprint.LegacyUnversionedPepperKeyID
		} else {
			return "", nil, fmt.Errorf("%s: api_key_active_pepper_key_id is required", op)
		}
	}

	if err := fingerprint.ValidatePepperKeyID(activeID); err != nil {
		return "", nil, fmt.Errorf("%s: api_key_active_pepper_key_id: %w", op, err)
	}

	if _, ok := peppers[activeID]; !ok {
		return "", nil, fmt.Errorf("%s: active pepper key id %q is not in the key ring", op, activeID)
	}

	return activeID, peppers, nil
}

// IssuanceEncryptionConfig returns the issuance secret encryption settings.
// Decode errors never include key material.
func (m MerchantTerminal) IssuanceEncryptionConfig() (secretbox.Config, error) {
	const op = "config.MerchantTerminal.IssuanceEncryptionConfig"

	cfg := secretbox.Config{
		ActiveKeyID: m.IssuanceEncryptionActiveKeyID,
		Keys:        make(map[string][]byte, len(m.IssuanceEncryptionKeys)),
	}

	for _, key := range m.IssuanceEncryptionKeys {
		raw, err := decodeBase64Key(key.Secret.Reveal())
		if err != nil {
			return secretbox.Config{}, fmt.Errorf("%s: key %q: %w", op, key.ID, err)
		}

		cfg.Keys[key.ID] = raw
	}

	if cfg.ActiveKeyID == "" && len(cfg.Keys) == 1 {
		for id := range cfg.Keys {
			cfg.ActiveKeyID = id
		}
	}

	return cfg, nil
}

// NewIssuanceSecretBox constructs the AES-GCM box that protects issued API keys
// held for idempotent replay.
func (m MerchantTerminal) NewIssuanceSecretBox() (*secretbox.Box, error) {
	const op = "config.MerchantTerminal.NewIssuanceSecretBox"

	cfg, err := m.IssuanceEncryptionConfig()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	box, err := secretbox.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("%s: create secret box: %w", op, err)
	}

	return box, nil
}

// UseCaseConfig returns the merchant terminal use-case settings.
func (m MerchantTerminal) UseCaseConfig() merchantterminaluc.Config {
	return merchantterminaluc.Config{
		DefaultGrace:        m.Rotation.DefaultGracePeriod,
		MaxGrace:            m.Rotation.MaxGracePeriod,
		IssuanceDeliveryTTL: m.IssuanceDeliveryTTL,
	}
}

func (m MerchantTerminal) newTerminalUseCase(
	pool repo.Pool,
	merchantProfile repo.MerchantProfile,
	merchantTerminal repo.MerchantTerminal,
	hasher fingerprint.Hasher,
	box coresecretbox.Box,
	clk clock.Clock,
) (*merchantterminaluc.UseCase, error) {
	return merchantterminaluc.New(
		pool,
		merchantProfile,
		merchantTerminal,
		hasher,
		box,
		clk,
		m.UseCaseConfig(),
	)
}

// validate checks pepper ring, grace periods, delivery TTL, and issuance
// encryption keys. Messages never include secret material.
func (m MerchantTerminal) validate() error {
	const op = "config.MerchantTerminal.validate"

	_, err := m.NewHasher()
	if err != nil {
		return fmt.Errorf("%s: invalid usecases.merchant_terminal api key pepper settings: %w", op, err)
	}

	if m.Rotation.DefaultGracePeriod <= 0 {
		return fmt.Errorf("%s: invalid usecases.merchant_terminal.rotation.default_grace_period: must be positive", op)
	}

	if m.Rotation.MaxGracePeriod <= 0 {
		return fmt.Errorf("%s: invalid usecases.merchant_terminal.rotation.max_grace_period: must be positive", op)
	}

	if m.Rotation.DefaultGracePeriod > m.Rotation.MaxGracePeriod {
		return fmt.Errorf(
			"%s: invalid usecases.merchant_terminal.rotation.default_grace_period: must not exceed max_grace_period", op,
		)
	}

	if m.IssuanceDeliveryTTL <= 0 {
		return fmt.Errorf("%s: invalid usecases.merchant_terminal.issuance_delivery_ttl: must be positive", op)
	}

	_, err = m.NewIssuanceSecretBox()
	if err != nil {
		return fmt.Errorf("%s: invalid usecases.merchant_terminal issuance encryption settings: %w", op, err)
	}

	return nil
}
