package config

import (
	"fmt"

	fingerprinthmac "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/fingerprint/hmac"
	"github.com/parstis/bnpl-backend/pkg/core/fingerprint"
)

// PolicyAssignment configures operational policy-assignment import commands.
type PolicyAssignment struct {
	// MobileHashActivePepperKeyID selects the pepper used to HMAC newly imported
	// trusted-customer mobiles. It must exist in MobileHashPepperKeys.
	MobileHashActivePepperKeyID string `yaml:"mobile_hash_active_pepper_key_id"`

	// MobileHashPepperKeys is the versioned pepper ring for mobile HMAC
	// fingerprints persisted on policy_assignment_import_items.mobile_hash.
	MobileHashPepperKeys KeyEntries `yaml:"mobile_hash_pepper_keys"`
}

// NewMobileHasher constructs the peppered HMAC hasher used to fingerprint
// canonical E.164 mobiles during trusted-customer import.
func (p PolicyAssignment) NewMobileHasher() (*fingerprinthmac.Hasher, error) {
	const op = "config.PolicyAssignment.NewMobileHasher"

	activeID, peppers, err := p.resolveMobileHashPepperRing()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	hasher, err := fingerprinthmac.NewKeyRing(activeID, peppers)
	if err != nil {
		return nil, fmt.Errorf("%s: create hasher: %w", op, err)
	}

	return hasher, nil
}

func (p PolicyAssignment) resolveMobileHashPepperRing() (string, map[string][]byte, error) {
	const op = "config.PolicyAssignment.resolveMobileHashPepperRing"

	if len(p.MobileHashPepperKeys) == 0 {
		return "", nil, fmt.Errorf("%s: mobile_hash_pepper_keys is required", op)
	}

	peppers := make(map[string][]byte, len(p.MobileHashPepperKeys))
	for _, entry := range p.MobileHashPepperKeys {
		if err := fingerprint.ValidatePepperKeyID(entry.ID); err != nil {
			return "", nil, fmt.Errorf("%s: mobile_hash_pepper_keys entry: %w", op, err)
		}

		if _, exists := peppers[entry.ID]; exists {
			return "", nil, fmt.Errorf("%s: duplicate mobile_hash_pepper_keys id %q", op, entry.ID)
		}

		if entry.Secret.Reveal() == "" {
			return "", nil, fmt.Errorf("%s: mobile_hash_pepper_keys entry %q secret is required", op, entry.ID)
		}

		peppers[entry.ID] = []byte(entry.Secret.Reveal())
	}

	activeID := p.MobileHashActivePepperKeyID
	if activeID == "" {
		if len(peppers) == 1 {
			for id := range peppers {
				activeID = id
			}
		} else {
			return "", nil, fmt.Errorf("%s: mobile_hash_active_pepper_key_id is required", op)
		}
	}

	if err := fingerprint.ValidatePepperKeyID(activeID); err != nil {
		return "", nil, fmt.Errorf("%s: mobile_hash_active_pepper_key_id: %w", op, err)
	}

	if _, ok := peppers[activeID]; !ok {
		return "", nil, fmt.Errorf("%s: active pepper key id %q is not in the key ring", op, activeID)
	}

	return activeID, peppers, nil
}

func (p PolicyAssignment) validate() error {
	const op = "config.PolicyAssignment.validate"

	if _, err := p.NewMobileHasher(); err != nil {
		return fmt.Errorf("%s: invalid usecases.policy_assignment mobile hash pepper settings: %w", op, err)
	}

	return nil
}
