package config

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	identityext "github.com/parstis/bnpl-backend/pkg/adapter/external/identity"
	identitygrpc "github.com/parstis/bnpl-backend/pkg/adapter/external/identity/grpc"
	identitynoop "github.com/parstis/bnpl-backend/pkg/adapter/external/identity/noop"
	"github.com/parstis/bnpl-backend/pkg/core/circuitbreaker"
	extidentity "github.com/parstis/bnpl-backend/pkg/core/external/identity"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// ExternalIdentity configures the Identity Service integration used by hosted
// external checkout for mobile → subject resolution.
type ExternalIdentity struct {
	// Provider selects the Identity Service client implementation.
	Provider string `yaml:"provider"`

	// Noop contains configuration for the noop Identity Service provider.
	Noop ExternalIdentityNoop `yaml:"noop"`

	// GRPC configures the Identity Service gRPC client.
	GRPC ExternalIdentityGRPC `yaml:"grpc"`
}

// ExternalIdentityNoop holds noop provider settings.
type ExternalIdentityNoop struct{}

// ExternalIdentityGRPC configures the Identity Service gRPC client.
type ExternalIdentityGRPC struct {
	// Address is the Identity Service gRPC server address.
	Address string `yaml:"address"`

	// Timeout configures the per-RPC timeout for Identity Service gRPC calls.
	Timeout time.Duration `yaml:"timeout"`

	// Insecure controls whether TLS is disabled for the Identity Service gRPC
	// connection. Production must keep this false; mTLS is configured later.
	Insecure bool `yaml:"insecure"`
}

// NewExternalIdentityClient constructs the configured Identity Service resolver.
// The optional closer owns the gRPC connection.
func (c *Config) NewExternalIdentityClient(
	breaker circuitbreaker.CircuitBreaker,
) (extidentity.Resolver, io.Closer, error) {
	return c.External.Identity.newClient(breaker)
}

func (e ExternalIdentity) newClient(
	breaker circuitbreaker.CircuitBreaker,
) (extidentity.Resolver, io.Closer, error) {
	const op = "config.ExternalIdentity.newClient"

	provider := identityext.Provider(e.Provider)

	switch provider {
	case identityext.GRPCProvider:
		return e.newGRPCClient(breaker)

	case identityext.NoopProvider:
		return identitynoop.New(), nil, nil

	default:
		return nil, nil, fmt.Errorf("%s: unsupported external identity provider: %q", op, e.Provider)
	}
}

func (e ExternalIdentity) newGRPCClient(
	breaker circuitbreaker.CircuitBreaker,
) (extidentity.Resolver, io.Closer, error) {
	const op = "config.ExternalIdentity.newGRPCClient"

	if strings.TrimSpace(e.GRPC.Address) == "" {
		return nil, nil, fmt.Errorf("%s: invalid external.identity.grpc.address: empty", op)
	}

	conn, err := identitygrpc.Dial(e.GRPC.Address, e.GRPC.Insecure)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: initialize external identity grpc client: %w", op, err)
	}

	client, err := identitygrpc.New(conn, e.GRPC.Timeout, breaker)
	if err != nil {
		return nil, nil, errors.Join(
			fmt.Errorf("%s: construct external identity grpc client: %w", op, err),
			conn.Close(),
		)
	}

	return client, conn, nil
}

func (e ExternalIdentity) validate(env model.RuntimeEnvironment) error {
	const op = "config.ExternalIdentity.validate"

	provider := identityext.Provider(e.Provider)

	switch provider {
	case identityext.NoopProvider:
		if isProductionMode(env) {
			return fmt.Errorf("%s: external identity provider \"noop\" is not allowed in production", op)
		}

	case identityext.GRPCProvider:
		if strings.TrimSpace(e.GRPC.Address) == "" {
			return fmt.Errorf("%s: invalid external.identity.grpc.address: value is required", op)
		}

		if e.GRPC.Timeout <= 0 {
			return fmt.Errorf("%s: invalid external.identity.grpc.timeout: must be positive", op)
		}

		if isProductionMode(env) && e.GRPC.Insecure {
			return fmt.Errorf("%s: external.identity.grpc.insecure must be false in production", op)
		}

	default:
		return fmt.Errorf("%s: invalid external.identity.provider: unsupported value %q", op, e.Provider)
	}

	return nil
}
