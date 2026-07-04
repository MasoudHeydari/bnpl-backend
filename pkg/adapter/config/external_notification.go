package config

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	notificationext "github.com/parstis/bnpl-backend/pkg/adapter/external/notification"
	notificationgrpc "github.com/parstis/bnpl-backend/pkg/adapter/external/notification/grpc"
	notificationnoop "github.com/parstis/bnpl-backend/pkg/adapter/external/notification/noop"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/grpcclient"
	"github.com/parstis/bnpl-backend/pkg/core/circuitbreaker"
	extnotification "github.com/parstis/bnpl-backend/pkg/core/external/notification"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// ExternalNotification configures the Notification Service integration used by
// hosted external checkout OTP submission.
type ExternalNotification struct {
	// Provider selects the Notification Service client implementation.
	Provider string `yaml:"provider"`

	// Noop contains configuration for the noop Notification Service provider.
	Noop ExternalNotificationNoop `yaml:"noop"`

	// GRPC configures the Notification Service gRPC client.
	GRPC ExternalNotificationGRPC `yaml:"grpc"`
}

// ExternalNotificationNoop holds noop provider settings.
type ExternalNotificationNoop struct{}

// ExternalNotificationGRPC configures the Notification Service gRPC client.
type ExternalNotificationGRPC struct {
	// Address is the Notification Service gRPC server address.
	Address string `yaml:"address"`

	// Timeout configures the per-RPC timeout for Notification Service gRPC calls.
	Timeout time.Duration `yaml:"timeout"`

	// ConnectionTimeout is the startup budget for verifying the gRPC transport
	// reaches READY after dial (before business RPCs).
	ConnectionTimeout time.Duration `yaml:"connection_timeout"`

	// Insecure disables TLS for the Notification Service gRPC connection.
	// Local plaintext only; production must keep this false.
	Insecure bool `yaml:"insecure"`

	// TLS configures server verification and optional mTLS when Insecure is false.
	TLS ExternalNotificationGRPCTLS `yaml:"tls"`
}

// ExternalNotificationGRPCTLS configures TLS materials for Notification gRPC.
type ExternalNotificationGRPCTLS struct {
	// CAFile is a PEM bundle used to verify the server certificate.
	CAFile string `yaml:"ca_file"`

	// CertFile is the optional client certificate PEM for mTLS.
	CertFile string `yaml:"cert_file"`

	// KeyFile is the optional client private key PEM for mTLS.
	KeyFile string `yaml:"key_file"`

	// ServerName overrides TLS hostname verification (SNI / cert name).
	ServerName string `yaml:"server_name"`
}

// NewExternalNotificationClient constructs the configured Notification Service
// submitter. The optional closer owns the gRPC connection.
func (c *Config) NewExternalNotificationClient(
	breaker circuitbreaker.CircuitBreaker,
) (extnotification.Submitter, io.Closer, error) {
	return c.External.Notification.newClient(breaker)
}

func (e ExternalNotification) newClient(
	breaker circuitbreaker.CircuitBreaker,
) (extnotification.Submitter, io.Closer, error) {
	const op = "config.ExternalNotification.newClient"

	provider := notificationext.Provider(e.Provider)

	switch provider {
	case notificationext.GRPCProvider:
		return e.newGRPCClient(breaker)

	case notificationext.NoopProvider:
		return notificationnoop.New(), nil, nil

	default:
		return nil, nil, fmt.Errorf("%s: unsupported external notification provider: %q", op, e.Provider)
	}
}

func (e ExternalNotification) newGRPCClient(
	breaker circuitbreaker.CircuitBreaker,
) (extnotification.Submitter, io.Closer, error) {
	const op = "config.ExternalNotification.newGRPCClient"

	if strings.TrimSpace(e.GRPC.Address) == "" {
		return nil, nil, fmt.Errorf("%s: invalid external.notification.grpc.address: empty", op)
	}

	conn, err := notificationgrpc.Dial(e.GRPC.Address, e.GRPC.Insecure, notificationgrpc.TLSConfig{
		CAFile:     e.GRPC.TLS.CAFile,
		CertFile:   e.GRPC.TLS.CertFile,
		KeyFile:    e.GRPC.TLS.KeyFile,
		ServerName: e.GRPC.TLS.ServerName,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("%s: initialize external notification grpc client: %w", op, err)
	}

	pingCtx, cancel := context.WithTimeout(context.Background(), e.GRPC.ConnectionTimeout)
	defer cancel()

	if err := grpcclient.Ping(pingCtx, conn); err != nil {
		return nil, nil, errors.Join(
			fmt.Errorf("%s: notification grpc service %q unavailable: %w", op, e.GRPC.Address, err),
			conn.Close(),
		)
	}

	client, err := notificationgrpc.New(conn, e.GRPC.Timeout, breaker)
	if err != nil {
		return nil, nil, errors.Join(
			fmt.Errorf("%s: construct external notification grpc client: %w", op, err),
			conn.Close(),
		)
	}

	return client, conn, nil
}

func (e ExternalNotification) validate(env model.RuntimeEnvironment) error {
	const op = "config.ExternalNotification.validate"

	provider := notificationext.Provider(e.Provider)

	switch provider {
	case notificationext.NoopProvider:
		if isProductionMode(env) {
			return fmt.Errorf("%s: external notification provider \"noop\" is not allowed in production", op)
		}

	case notificationext.GRPCProvider:
		if strings.TrimSpace(e.GRPC.Address) == "" {
			return fmt.Errorf("%s: invalid external.notification.grpc.address: value is required", op)
		}

		if e.GRPC.Timeout <= 0 {
			return fmt.Errorf("%s: invalid external.notification.grpc.timeout: must be positive", op)
		}

		if e.GRPC.ConnectionTimeout <= 0 {
			return fmt.Errorf("%s: invalid external.notification.grpc.connection_timeout: must be positive", op)
		}

		if isProductionMode(env) && e.GRPC.Insecure {
			return fmt.Errorf("%s: external.notification.grpc.insecure must be false in production", op)
		}

		if !e.GRPC.Insecure {
			certFile := strings.TrimSpace(e.GRPC.TLS.CertFile)
			keyFile := strings.TrimSpace(e.GRPC.TLS.KeyFile)
			if (certFile == "") != (keyFile == "") {
				return fmt.Errorf(
					"%s: invalid external.notification.grpc.tls: cert_file and key_file must both be set or both empty",
					op,
				)
			}
		}

	default:
		return fmt.Errorf("%s: invalid external.notification.provider: unsupported value %q", op, e.Provider)
	}

	return nil
}
