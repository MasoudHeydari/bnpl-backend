package config

import (
	"encoding/base64"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

const (
	testSecretA = "key-entry-secret-a-0123456789abcd"
	testSecretB = "key-entry-secret-b-0123456789abcd"
)

func TestKeyEntries_UnmarshalYAML(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		doc  string
		want []string
	}{
		"scalar": {"keys: k1:" + testSecretA + ",k2:" + testSecretB, []string{"k1", "k2"}},
		"list": {
			"keys:\n  - id: k1\n    secret: " + testSecretA + "\n  - id: k2\n    secret: " + testSecretB,
			[]string{"k1", "k2"},
		},
		"empty scalar": {"keys: ", nil},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var out struct {
				Keys KeyEntries `yaml:"keys"`
			}

			if err := yaml.Unmarshal([]byte(tc.doc), &out); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			var got []string
			for _, k := range out.Keys {
				got = append(got, k.ID)
			}

			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("ids = %v, want %v", got, tc.want)
			}
		})
	}

	var bad struct {
		Keys KeyEntries `yaml:"keys"`
	}

	if err := yaml.Unmarshal([]byte("keys: no-colon"), &bad); err == nil {
		t.Fatal("expected error for entry without colon")
	}
}

func TestCustomerPhoneEncryptionConfig(t *testing.T) {
	t.Parallel()

	key := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("p", 32)))
	valid := CheckoutSession{
		WebhookSecretEncryptionKeys: KeyEntries{
			{ID: "w1", Secret: model.NewSecret(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))))},
		},
		CustomerPhoneEncryptionKeys: KeyEntries{
			{ID: "p1", Secret: model.NewSecret(key)},
		},
	}

	cfg, err := valid.CustomerPhoneEncryptionConfig()
	if err != nil {
		t.Fatalf("CustomerPhoneEncryptionConfig: %v", err)
	}

	if cfg.ActiveKeyID != "p1" {
		t.Fatalf("unexpected active key: %q", cfg.ActiveKeyID)
	}

	if err = valid.validateSecrets(); err != nil {
		t.Fatalf("validate: %v", err)
	}

	box, err := valid.NewCustomerPhoneBox()
	if err != nil {
		t.Fatalf("NewCustomerPhoneBox: %v", err)
	}

	ciphertext, keyID, err := box.Encrypt("+989121234567")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	plaintext, err := box.Decrypt(ciphertext, keyID)
	if err != nil || plaintext != "+989121234567" {
		t.Fatalf("round trip: %q, %v", plaintext, err)
	}

	missing := valid
	missing.CustomerPhoneEncryptionKeys = nil

	if err = missing.validateSecrets(); err == nil {
		t.Fatal("expected error without customer phone encryption keys")
	}

	orphan := valid
	orphan.CustomerPhoneEncryptionActiveKeyID = "nope"

	if err = orphan.validateSecrets(); err == nil {
		t.Fatal("expected error for unknown active key")
	}
}

func TestWebhookSecretEncryptionConfig(t *testing.T) {
	t.Parallel()

	key := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))
	base := CheckoutSession{
		WebhookSecretEncryptionKeys: KeyEntries{
			{ID: "w1", Secret: model.NewSecret(key)},
		},
		CustomerPhoneEncryptionKeys: KeyEntries{
			{ID: "p1", Secret: model.NewSecret(key)},
		},
	}

	cfg, err := base.WebhookSecretEncryptionConfig()
	if err != nil {
		t.Fatalf("WebhookSecretEncryptionConfig: %v", err)
	}

	if cfg.ActiveKeyID != "w1" || len(cfg.Keys["w1"]) != 32 {
		t.Fatalf("unexpected config: active=%q", cfg.ActiveKeyID)
	}

	if err = base.validateSecrets(); err != nil {
		t.Fatalf("validate: %v", err)
	}

	box, err := base.NewWebhookSecretBox()
	if err != nil {
		t.Fatalf("NewWebhookSecretBox: %v", err)
	}

	ciphertext, keyID, err := box.Encrypt("whsec")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	if plaintext, err := box.Decrypt(ciphertext, keyID); err != nil || plaintext != "whsec" {
		t.Fatalf("round trip: %q, %v", plaintext, err)
	}

	missing := CheckoutSession{}
	if err = missing.validateSecrets(); err == nil {
		t.Fatal("expected error without webhook encryption keys")
	}

	orphan := base
	orphan.WebhookSecretEncryptionActiveKeyID = "nope"

	if err = orphan.validateSecrets(); err == nil {
		t.Fatal("expected error for unknown active key")
	}

	for name, material := range map[string]string{
		"short":      "c2hvcnQ",
		"not base64": "%%%not-base64%%%",
	} {
		bad := base
		bad.WebhookSecretEncryptionKeys = KeyEntries{{ID: "w1", Secret: model.NewSecret(material)}}

		err = bad.validateSecrets()
		if err == nil {
			t.Fatalf("%s: expected error", name)
		}

		if strings.Contains(err.Error(), material) {
			t.Fatalf("%s: error leaks key material", name)
		}
	}
}
