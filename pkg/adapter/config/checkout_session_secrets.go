package config

import (
	"encoding/base64"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/secretbox"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// KeyEntry is one named secret in a key set.
type KeyEntry struct {
	// ID names the key.
	ID string `yaml:"id"`

	// Secret is the key material. It is redacted in logs and marshaled output.
	Secret model.Secret `yaml:"secret"`
}

// KeyEntries is a set of named secrets.
//
// In YAML it may be written either as a list of {id, secret} mappings or as a
// single scalar "id:secret,id2:secret2". The scalar form exists so the whole set
// can come from one environment variable.
type KeyEntries []KeyEntry

// UnmarshalYAML decodes either a sequence of {id, secret} or a scalar
// "id:secret,id2:secret2". Secrets in the scalar form must not contain commas.
func (k *KeyEntries) UnmarshalYAML(node *yaml.Node) error {
	const op = "config.KeyEntries.UnmarshalYAML"

	switch node.Kind {
	case yaml.ScalarNode:
		entries, err := parseKeyEntries(node.Value)
		if err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}

		*k = entries

		return nil

	case yaml.SequenceNode:
		var entries []KeyEntry

		err := node.Decode(&entries)
		if err != nil {
			return fmt.Errorf("%s: decode key entries: %w", op, err)
		}

		*k = entries

		return nil

	default:
		return fmt.Errorf("%s: key entries must be a list or an id:secret,... string", op)
	}
}

func parseKeyEntries(raw string) (KeyEntries, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}

	var entries KeyEntries

	for i, part := range strings.Split(raw, ",") {
		id, secret, ok := strings.Cut(strings.TrimSpace(part), ":")
		if !ok {
			return nil, fmt.Errorf("key entry %d must be id:secret", i)
		}

		entries = append(entries, KeyEntry{
			ID:     strings.TrimSpace(id),
			Secret: model.NewSecret(strings.TrimSpace(secret)),
		})
	}

	return entries, nil
}

// WebhookSecretEncryptionConfig returns the webhook secret encryption settings.
// Key secrets are base64 (standard or URL-safe) encoded 32-byte AES-256 keys.
// Decode errors never include key material.
func (c CheckoutSession) WebhookSecretEncryptionConfig() (secretbox.Config, error) {
	const op = "config.CheckoutSession.WebhookSecretEncryptionConfig"

	cfg, err := secretBoxConfig(c.WebhookSecretEncryptionActiveKeyID, c.WebhookSecretEncryptionKeys)
	if err != nil {
		return secretbox.Config{}, fmt.Errorf("%s: %w", op, err)
	}

	return cfg, nil
}

// NewWebhookSecretBox constructs the AES-GCM box that protects merchant webhook
// signing secrets at rest.
func (c CheckoutSession) NewWebhookSecretBox() (*secretbox.Box, error) {
	const op = "config.CheckoutSession.NewWebhookSecretBox"

	cfg, err := c.WebhookSecretEncryptionConfig()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	box, err := secretbox.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("%s: create secret box: %w", op, err)
	}

	return box, nil
}

// CustomerPhoneEncryptionConfig returns the customer phone encryption settings.
// Key secrets are base64 (standard or URL-safe) encoded 32-byte AES-256 keys.
// Decode errors never include key material.
func (c CheckoutSession) CustomerPhoneEncryptionConfig() (secretbox.Config, error) {
	const op = "config.CheckoutSession.CustomerPhoneEncryptionConfig"

	cfg, err := secretBoxConfig(c.CustomerPhoneEncryptionActiveKeyID, c.CustomerPhoneEncryptionKeys)
	if err != nil {
		return secretbox.Config{}, fmt.Errorf("%s: %w", op, err)
	}

	return cfg, nil
}

// NewCustomerPhoneBox constructs the AES-GCM box that protects the customer
// phone supplied on external partner checkout creation at rest.
func (c CheckoutSession) NewCustomerPhoneBox() (*secretbox.Box, error) {
	const op = "config.CheckoutSession.NewCustomerPhoneBox"

	cfg, err := c.CustomerPhoneEncryptionConfig()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	box, err := secretbox.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("%s: create secret box: %w", op, err)
	}

	return box, nil
}

// validateSecrets validates webhook secret and customer phone encryption settings.
func (c CheckoutSession) validateSecrets() error {
	const op = "config.CheckoutSession.validateSecrets"

	// Validation reuses the constructors so config and runtime rules cannot drift.
	if _, err := c.NewWebhookSecretBox(); err != nil {
		return fmt.Errorf("%s: invalid usecases.checkout_session webhook secret encryption settings: %w", op, err)
	}

	if _, err := c.NewCustomerPhoneBox(); err != nil {
		return fmt.Errorf("%s: invalid usecases.checkout_session customer phone encryption settings: %w", op, err)
	}

	return nil
}

// secretBoxConfig decodes a configured key set into a secretbox configuration.
// When no active key id is given and exactly one key is configured, that key is
// active.
func secretBoxConfig(activeKeyID string, keys KeyEntries) (secretbox.Config, error) {
	const op = "config.secretBoxConfig"

	cfg := secretbox.Config{
		ActiveKeyID: activeKeyID,
		Keys:        make(map[string][]byte, len(keys)),
	}

	for _, key := range keys {
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

func decodeBase64Key(encoded string) ([]byte, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(encoded), "=")

	for _, encoding := range []*base64.Encoding{base64.RawStdEncoding, base64.RawURLEncoding} {
		raw, err := encoding.DecodeString(trimmed)
		if err == nil {
			return raw, nil
		}
	}

	return nil, fmt.Errorf("key must be base64 encoded")
}
