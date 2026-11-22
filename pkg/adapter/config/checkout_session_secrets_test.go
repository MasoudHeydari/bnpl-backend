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
