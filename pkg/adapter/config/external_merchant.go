package config

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/parstis/bnpl-backend/internal/buildprofile"
	merchantext "github.com/parstis/bnpl-backend/pkg/adapter/external/merchant"
	merchantgrpc "github.com/parstis/bnpl-backend/pkg/adapter/external/merchant/grpc"
	merchantnoop "github.com/parstis/bnpl-backend/pkg/adapter/external/merchant/noop"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/grpcclient"
	"github.com/parstis/bnpl-backend/pkg/core/circuitbreaker"
	"github.com/parstis/bnpl-backend/pkg/core/external/merchant"
	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

const MerchantParticipationTopic = "merchant.business.line.participation.requests.v1"

// ExternalMerchant configures the private Merchant participation integration.
type ExternalMerchant struct {
	Provider string               `yaml:"provider"`
	Noop     ExternalMerchantNoop `yaml:"noop"`
	GRPC     ExternalMerchantGRPC `yaml:"grpc"`
}

type ExternalMerchantNoop struct{}

// ExternalMerchantGRPC holds the transport and authoritative environment binding.
type ExternalMerchantGRPC struct {
	Address           string                  `yaml:"address"`
	Timeout           time.Duration           `yaml:"timeout"`
	ConnectionTimeout time.Duration           `yaml:"connection_timeout"`
	Environment       string                  `yaml:"environment"`
	TLS               ExternalMerchantGRPCTLS `yaml:"tls"`
}

type ExternalMerchantGRPCTLS struct {
	CAFile     string `yaml:"ca_file"`
	CertFile   string `yaml:"cert_file"`
	KeyFile    string `yaml:"key_file"`
	ServerName string `yaml:"server_name"`
}

// NewExternalMerchantClient uses the same lifecycle and TLS infrastructure as other external services.
func (c *Config) NewExternalMerchantClient(breaker circuitbreaker.CircuitBreaker, logger log.Logger) (merchant.Client, io.Closer, error) {
	const op = "config.Config.NewExternalMerchantClient"

	e := c.External.Merchant
	switch merchantext.Provider(e.Provider) {
	case "", merchantext.NoopProvider:
		return merchantnoop.New(), nil, nil

	case merchantext.GRPCProvider:
		conn, err := merchantgrpc.Dial(e.GRPC.Address, merchantgrpc.TLSConfig{
			CAFile:     e.GRPC.TLS.CAFile,
			CertFile:   e.GRPC.TLS.CertFile,
			KeyFile:    e.GRPC.TLS.KeyFile,
			ServerName: e.GRPC.TLS.ServerName,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("%s: dial merchant service: %w", op, err)
		}

		pingCtx, cancel := context.WithTimeout(context.Background(), e.GRPC.ConnectionTimeout)
		defer cancel()

		if err := grpcclient.Ping(pingCtx, conn); err != nil {
			logger.With("address", e.GRPC.Address).Warn(log.Msg("merchant grpc service not ready at startup; continuing"))
		}

		client, err := merchantgrpc.New(conn, e.GRPC.Timeout, breaker)
		if err != nil {
			return nil, nil, errors.Join(fmt.Errorf("%s: construct merchant client: %w", op, err), conn.Close())
		}

		return client, conn, nil

	default:
		return nil, nil, fmt.Errorf("%s: unsupported external merchant provider %q", op, e.Provider)
	}
}

func (c *Config) validateMerchantParticipation(env model.RuntimeEnvironment) error {
	const op = "config.Config.validateMerchantParticipation"

	e := c.External.Merchant
	k := c.Infrastructure.Messaging.Kafka
	topic := k.Topics.MerchantParticipationRequests
	task := c.Infrastructure.Task.Scheduled.MerchantProgressDelivery
	subscribed := false
	for _, name := range k.Consumer.SubscriptionTopics() {
		if name != "" && (name == topic || name == MerchantParticipationTopic) {
			subscribed = true
		}
	}

	switch merchantext.Provider(e.Provider) {
	case "", merchantext.NoopProvider:
		if subscribed || task.Enabled {
			return fmt.Errorf("%s: participation subscription and delivery require external.merchant.provider=grpc", op)
		}

	case merchantext.GRPCProvider:
		if e.GRPC.Environment != "SANDBOX" && e.GRPC.Environment != "LIVE" {
			return fmt.Errorf("%s: invalid external.merchant.grpc.environment", op)
		}

		if e.GRPC.Environment == "SANDBOX" && (!buildprofile.IsDevelopment() || !env.IsDevelopment()) {
			return fmt.Errorf("%s: SANDBOX participation requires a development build and runtime", op)
		}

		if e.GRPC.Environment == "LIVE" {
			return fmt.Errorf("%s: LIVE participation is not supported by the current Merchant contract", op)
		}

		if strings.TrimSpace(e.GRPC.Address) == "" || e.GRPC.Timeout <= 0 || e.GRPC.ConnectionTimeout <= 0 ||
			strings.TrimSpace(e.GRPC.TLS.CAFile) == "" || strings.TrimSpace(e.GRPC.TLS.CertFile) == "" ||
			strings.TrimSpace(e.GRPC.TLS.KeyFile) == "" || strings.TrimSpace(e.GRPC.TLS.ServerName) == "" {
			return fmt.Errorf("%s: Merchant address, timeouts and all mTLS fields are required", op)
		}

		if c.Infrastructure.Messaging.Provider != "kafka" || topic != MerchantParticipationTopic || !subscribed || !task.Enabled {
			return fmt.Errorf("%s: Merchant gRPC requires its Kafka subscription and scheduled progress delivery", op)
		}

		if c.Infrastructure.Task.Provider == "noop" || task.Handler != "merchant_progress_delivery" {
			return fmt.Errorf("%s: Merchant gRPC requires an active progress delivery handler", op)
		}

		if c.Infrastructure.CircuitBreaker.Provider != "gobreaker" {
			return fmt.Errorf("%s: Merchant gRPC requires the gobreaker provider", op)
		}

	default:
		return fmt.Errorf("%s: invalid external.merchant.provider %q", op, e.Provider)
	}

	return nil
}
