package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"

	coresecretbox "github.com/parstis/bnpl-backend/pkg/core/secretbox"
)

// KeyLength is the required key size in bytes (AES-256).
const KeyLength = 32

const (
	ciphertextVersion = "sb1"
	aadDomain         = "bnpl-secretbox.v1\n"
	maxPlaintextLen   = 4096
)

var keyIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

var (
	// ErrDecryptFailed is returned when a ciphertext is malformed, was sealed
	// under another key, or fails authentication. It carries no detail on purpose.
	ErrDecryptFailed = errors.New("secretbox: cannot decrypt")

	// ErrUnknownKeyID is returned when no configured key matches the key id.
	ErrUnknownKeyID = errors.New("secretbox: unknown key id")
)

// Config configures a Box.
type Config struct {
	// ActiveKeyID selects the key used by Encrypt. It must exist in Keys.
	ActiveKeyID string

	// Keys maps key id to a raw KeyLength-byte AES-256 key. Every key can
	// decrypt; only ActiveKeyID encrypts.
	Keys map[string][]byte
}

// Box is an AES-256-GCM secret box.
type Box struct {
	active string
	aeads  map[string]cipher.AEAD
}

var _ coresecretbox.Box = (*Box)(nil)

// Validate checks that cfg describes a usable key set.
func (c Config) Validate() error {
	const op = "secretbox.Config.Validate"

	if len(c.Keys) == 0 {
		return fmt.Errorf("%s: at least one key is required", op)
	}

	for id, key := range c.Keys {
		if !keyIDPattern.MatchString(id) {
			return fmt.Errorf("%s: key id %q must match %s", op, id, keyIDPattern)
		}

		if len(key) != KeyLength {
			return fmt.Errorf("%s: key %q must be exactly %d bytes", op, id, KeyLength)
		}
	}

	if _, ok := c.Keys[c.ActiveKeyID]; !ok {
		return fmt.Errorf("%s: active key id %q is not in the key set", op, c.ActiveKeyID)
	}

	return nil
}

// New constructs a Box from cfg.
func New(cfg Config) (*Box, error) {
	const op = "secretbox.New"

	err := cfg.Validate()
	if err != nil {
		return nil, fmt.Errorf("%s: validate config: %w", op, err)
	}

	aeads := make(map[string]cipher.AEAD, len(cfg.Keys))

	for id, key := range cfg.Keys {
		block, blockErr := aes.NewCipher(key)
		if blockErr != nil {
			return nil, fmt.Errorf("%s: create cipher for key %q: %w", op, id, blockErr)
		}

		aead, gcmErr := cipher.NewGCM(block)
		if gcmErr != nil {
			return nil, fmt.Errorf("%s: create gcm for key %q: %w", op, id, gcmErr)
		}

		aeads[id] = aead
	}

	return &Box{active: cfg.ActiveKeyID, aeads: aeads}, nil
}

// Encrypt seals plaintext with the active key.
func (b *Box) Encrypt(plaintext string) (string, string, error) {
	const op = "secretbox.Box.Encrypt"

	if plaintext == "" {
		return "", "", fmt.Errorf("%s: plaintext must not be empty", op)
	}

	if len(plaintext) > maxPlaintextLen {
		return "", "", fmt.Errorf("%s: plaintext exceeds %d bytes", op, maxPlaintextLen)
	}

	aead := b.aeads[b.active]

	nonce := make([]byte, aead.NonceSize())

	_, err := rand.Read(nonce)
	if err != nil {
		return "", "", fmt.Errorf("%s: generate nonce: %w", op, err)
	}

	sealed := aead.Seal(nonce, nonce, []byte(plaintext), additionalData(b.active))

	return ciphertextVersion + "." + base64.RawURLEncoding.EncodeToString(sealed), b.active, nil
}

// Decrypt opens ciphertext using the key identified by keyID.
func (b *Box) Decrypt(ciphertext string, keyID string) (string, error) {
	const op = "secretbox.Box.Decrypt"

	aead, ok := b.aeads[keyID]
	if !ok {
		return "", fmt.Errorf("%s: %w", op, ErrUnknownKeyID)
	}

	encoded, found := strings.CutPrefix(ciphertext, ciphertextVersion+".")
	if !found {
		return "", fmt.Errorf("%s: unsupported ciphertext version: %w", op, ErrDecryptFailed)
	}

	sealed, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("%s: malformed ciphertext: %w", op, ErrDecryptFailed)
	}

	if len(sealed) < aead.NonceSize()+aead.Overhead() {
		return "", fmt.Errorf("%s: ciphertext too short: %w", op, ErrDecryptFailed)
	}

	nonce, body := sealed[:aead.NonceSize()], sealed[aead.NonceSize():]

	plaintext, err := aead.Open(nil, nonce, body, additionalData(keyID))
	if err != nil {
		return "", fmt.Errorf("%s: authentication failed: %w", op, ErrDecryptFailed)
	}

	return string(plaintext), nil
}

func additionalData(keyID string) []byte {
	return []byte(aadDomain + keyID)
}
