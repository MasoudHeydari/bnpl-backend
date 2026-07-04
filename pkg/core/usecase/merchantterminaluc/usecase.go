package merchantterminaluc

import (
	"crypto/rand"
	"fmt"
	"io"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/clock"
	"github.com/parstis/bnpl-backend/pkg/core/fingerprint"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/secretbox"
)

const (
	// provisionMaxAttempts bounds how often Provision restarts after a terminal
	// id collision or a concurrent provisioning of the same merchant.
	provisionMaxAttempts = 5

	// maxIdempotencyKeyLength bounds the caller-supplied idempotency key.
	maxIdempotencyKeyLength = 128

	// Operation prefixes keep Provision and Rotate idempotency keys in separate
	// namespaces within one terminal.
	provisionKeyPrefix = "provision:"
	rotateKeyPrefix    = "rotate:"
)

// Config tunes credential rotation and issuance replay.
type Config struct {
	// DefaultGrace is how long a rotated credential stays accepted when the
	// caller does not choose a grace period.
	DefaultGrace time.Duration

	// MaxGrace is the longest grace period a caller may request.
	MaxGrace time.Duration

	// IssuanceDeliveryTTL is how long an issued API key can be replayed to an
	// idempotent retry.
	IssuanceDeliveryTTL time.Duration
}

// Validate checks that cfg describes usable durations.
func (c Config) Validate() error {
	const op = "merchantterminaluc.Config.Validate"

	if c.DefaultGrace < 0 {
		return fmt.Errorf("%s: default grace must not be negative", op)
	}

	if c.MaxGrace < c.DefaultGrace {
		return fmt.Errorf("%s: max grace must not be less than default grace", op)
	}

	if c.IssuanceDeliveryTTL <= 0 {
		return fmt.Errorf("%s: issuance delivery ttl must be positive", op)
	}

	return nil
}

// UseCase authenticates partner terminals and manages their credentials.
//
// It records no business metrics, so it takes no metrics dependency.
type UseCase struct {
	pool             repo.Pool
	merchantProfile  repo.MerchantProfile
	merchantTerminal repo.MerchantTerminal
	hasher           fingerprint.Hasher
	box              secretbox.Box
	clock            clock.Clock
	random           io.Reader
	cfg              Config
}

// New constructs a merchant terminal use case. box encrypts issued API keys
// for idempotent replay. hasher must be a keyed digest suitable for secret
// verification (typically the HMAC fingerprint adapter).
func New(
	pool repo.Pool,
	merchantProfile repo.MerchantProfile,
	merchantTerminal repo.MerchantTerminal,
	hasher fingerprint.Hasher,
	box secretbox.Box,
	clk clock.Clock,
	cfg Config,
) (*UseCase, error) {
	const op = "merchantterminaluc.New"

	if pool == nil {
		return nil, fmt.Errorf("%s: pool is required", op)
	}

	if merchantProfile == nil {
		return nil, fmt.Errorf("%s: merchant profile repository is required", op)
	}

	if merchantTerminal == nil {
		return nil, fmt.Errorf("%s: merchant terminal repository is required", op)
	}

	if hasher == nil {
		return nil, fmt.Errorf("%s: hasher is required", op)
	}

	if box == nil {
		return nil, fmt.Errorf("%s: secret box is required", op)
	}

	if clk == nil {
		return nil, fmt.Errorf("%s: clock is required", op)
	}

	err := cfg.Validate()
	if err != nil {
		return nil, fmt.Errorf("%s: validate config: %w", op, err)
	}

	return &UseCase{
		pool:             pool,
		merchantProfile:  merchantProfile,
		merchantTerminal: merchantTerminal,
		hasher:           hasher,
		box:              box,
		clock:            clk,
		random:           rand.Reader,
		cfg:              cfg,
	}, nil
}
