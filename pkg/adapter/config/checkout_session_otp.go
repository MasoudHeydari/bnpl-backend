package config

import (
	"fmt"
	"strings"

	fingerprinthmac "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/fingerprint/hmac"
	"github.com/parstis/bnpl-backend/pkg/core/fingerprint"
)

const (
	// DefaultCheckoutOTPTemplateID is the SMS template used when
	// otp_template_id is unset.
	DefaultCheckoutOTPTemplateID = "bnpl.external_checkout_otp.v1"
)

// EffectiveOTPTemplateID returns the configured SMS template id or its default.
func (c CheckoutSession) EffectiveOTPTemplateID() string {
	if strings.TrimSpace(c.OTPTemplateID) == "" {
		return DefaultCheckoutOTPTemplateID
	}

	return strings.TrimSpace(c.OTPTemplateID)
}

// NewOTPHasher constructs the versioned peppered HMAC hasher for checkout OTP
// digests. Pepper material never appears in returned errors.
func (c CheckoutSession) NewOTPHasher() (*fingerprinthmac.Hasher, error) {
	const op = "config.CheckoutSession.NewOTPHasher"

	activeID, peppers, err := c.resolveOTPPepperRing()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	hasher, err := fingerprinthmac.NewKeyRing(activeID, peppers)
	if err != nil {
		return nil, fmt.Errorf("%s: create hasher: %w", op, err)
	}

	return hasher, nil
}

func (c CheckoutSession) resolveOTPPepperRing() (string, map[string][]byte, error) {
	const op = "config.CheckoutSession.resolveOTPPepperRing"

	if len(c.OTPPepperKeys) == 0 {
		return "", nil, fmt.Errorf("%s: otp_pepper_keys is required", op)
	}

	peppers := make(map[string][]byte, len(c.OTPPepperKeys))

	for _, entry := range c.OTPPepperKeys {
		if err := fingerprint.ValidatePepperKeyID(entry.ID); err != nil {
			return "", nil, fmt.Errorf("%s: otp_pepper_keys entry: %w", op, err)
		}

		if _, exists := peppers[entry.ID]; exists {
			return "", nil, fmt.Errorf("%s: duplicate otp_pepper_keys id %q", op, entry.ID)
		}

		if entry.Secret.Reveal() == "" {
			return "", nil, fmt.Errorf("%s: otp_pepper_keys entry %q secret is required", op, entry.ID)
		}

		peppers[entry.ID] = []byte(entry.Secret.Reveal())
	}

	activeID := c.OTPActivePepperKeyID
	if activeID == "" {
		if len(peppers) != 1 {
			return "", nil, fmt.Errorf("%s: otp_active_pepper_key_id is required", op)
		}

		for id := range peppers {
			activeID = id
		}
	}

	if err := fingerprint.ValidatePepperKeyID(activeID); err != nil {
		return "", nil, fmt.Errorf("%s: otp_active_pepper_key_id: %w", op, err)
	}

	if _, ok := peppers[activeID]; !ok {
		return "", nil, fmt.Errorf("%s: active otp pepper key id %q is not in the key ring", op, activeID)
	}

	return activeID, peppers, nil
}

// validateOTP validates external checkout OTP settings. Validation reuses the
// constructors so config and runtime rules cannot drift.
func (c CheckoutSession) validateOTP() error {
	const op = "config.CheckoutSession.validateOTP"

	if _, err := c.NewOTPHasher(); err != nil {
		return fmt.Errorf("%s: invalid usecases.checkout_session otp pepper settings: %w", op, err)
	}

	return nil
}
