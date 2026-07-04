package config

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	customerext "github.com/parstis/bnpl-backend/pkg/adapter/external/customer"
	customergrpc "github.com/parstis/bnpl-backend/pkg/adapter/external/customer/grpc"
	customernoop "github.com/parstis/bnpl-backend/pkg/adapter/external/customer/noop"
	"github.com/parstis/bnpl-backend/pkg/core/circuitbreaker"
	extcustomer "github.com/parstis/bnpl-backend/pkg/core/external/customer"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// ExternalCustomer configures the Customer Reference Service integration used
// by Partner checkout for mobile → external customer_id resolution.
type ExternalCustomer struct {
	// Provider selects the Customer Reference Service client implementation.
	Provider string `yaml:"provider"`

	// Noop contains configuration for the noop Customer Reference provider.
	Noop ExternalCustomerNoop `yaml:"noop"`

	// GRPC configures the Customer Reference Service gRPC client.
	GRPC ExternalCustomerGRPC `yaml:"grpc"`
}

// ExternalCustomerNoop holds noop provider settings.
type ExternalCustomerNoop struct{}

// ExternalCustomerGRPC configures the Customer Reference Service gRPC client.
type ExternalCustomerGRPC struct {
	// Address is the Customer Reference Service gRPC server address.
	Address string `yaml:"address"`

	// Timeout configures the per-RPC timeout for Customer Reference gRPC calls.
	Timeout time.Duration `yaml:"timeout"`

	// Insecure controls whether TLS is disabled for the Customer Reference
	// gRPC connection. Production must keep this false; mTLS is configured later.
	Insecure bool `yaml:"insecure"`
}

// NewExternalCustomerClient constructs the configured Customer Reference
// Service resolver. The optional closer owns the gRPC connection.
func (c *Config) NewExternalCustomerClient(
	breaker circuitbreaker.CircuitBreaker,
) (extcustomer.Resolver, io.Closer, error) {
	return c.External.Customer.newClient(breaker)
}

func (e ExternalCustomer) newClient(
	breaker circuitbreaker.CircuitBreaker,
) (extcustomer.Resolver, io.Closer, error) {
	const op = "config.ExternalCustomer.newClient"

	provider := customerext.Provider(e.Provider)

	switch provider {
	case customerext.GRPCProvider:
		return e.newGRPCClient(breaker)

	case customerext.NoopProvider:
		return customernoop.New(), nil, nil

	default:
		return nil, nil, fmt.Errorf("%s: unsupported external customer provider: %q", op, e.Provider)
	}
}

func (e ExternalCustomer) newGRPCClient(
	breaker circuitbreaker.CircuitBreaker,
) (extcustomer.Resolver, io.Closer, error) {
	const op = "config.ExternalCustomer.newGRPCClient"

	if strings.TrimSpace(e.GRPC.Address) == "" {
		return nil, nil, fmt.Errorf("%s: invalid external.customer.grpc.address: empty", op)
	}

	conn, err := customergrpc.Dial(e.GRPC.Address, e.GRPC.Insecure)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: initialize external customer grpc client: %w", op, err)
	}

	client, err := customergrpc.New(conn, e.GRPC.Timeout, breaker)
	if err != nil {
		return nil, nil, errors.Join(
			fmt.Errorf("%s: construct external customer grpc client: %w", op, err),
			conn.Close(),
		)
	}

	return client, conn, nil
}

func (e ExternalCustomer) validate(env model.RuntimeEnvironment) error {
	const op = "config.ExternalCustomer.validate"

	provider := customerext.Provider(e.Provider)

	switch provider {
	case customerext.NoopProvider:
		if isProductionMode(env) {
			return fmt.Errorf("%s: external customer provider \"noop\" is not allowed in production", op)
		}

	case customerext.GRPCProvider:
		if strings.TrimSpace(e.GRPC.Address) == "" {
			return fmt.Errorf("%s: invalid external.customer.grpc.address: value is required", op)
		}

		if e.GRPC.Timeout <= 0 {
			return fmt.Errorf("%s: invalid external.customer.grpc.timeout: must be positive", op)
		}

		if isProductionMode(env) && e.GRPC.Insecure {
			return fmt.Errorf("%s: external.customer.grpc.insecure must be false in production", op)
		}

	default:
		return fmt.Errorf("%s: invalid external.customer.provider: unsupported value %q", op, e.Provider)
	}

	return nil
}
