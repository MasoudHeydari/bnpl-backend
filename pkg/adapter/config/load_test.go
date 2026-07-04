package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validVersionsYAML = `
versions:
  database: 0.2.0
  config: 0.3.0
`

const validMinimalValidationYAML = `
application: runtime
runtime:
  environment: development
domain:
  supported_jurisdictions:
    - IR
database:
  host: localhost
  port: 5432
  name: bnpl
  pgpass_entries:
    normal: localhost:5432:bnpl:bnpl:secret
usecases:
  checkout_session:
    public_origin: https://bnpl.example
    public_origin_policy:
      allowed_hosts:
        - bnpl.example
    ttl: 30m
    expiry_batch_size: 100
    lifecycle_events_destination: bnpl.merchant.checkout.events.v1
    webhook_secret_encryption_keys: w1:ZGV2LW9ubHktd2ViaG9vay1lbmNyeXB0aW9uLWtleSE=
    otp_pepper_keys: o1:test-checkout-otp-pepper-0123456789ab
    customer_phone_encryption_keys: p1:ZGV2LW9ubHktY3VzdG9tZXItcGhvbmUta2V5LTAxMjM=
  outbox_publisher:
    worker_id: worker-1
    batch_size: 10
    retry_base_delay: 1s
    retry_max_delay: 1m
    processing_lease_timeout: 5m
  installment:
    mark_due_batch_size: 100
    mark_overdue_batch_size: 100
  contract:
    payment_result_processing_grace_period: 5m
    expiry_batch_size: 100
  payment_attempt_reconciliation:
    batch_size: 100
    created_timeout: 5m
    retry_base_delay: 1s
    retry_max_delay: 1m
    max_attempts: 10
  merchant_terminal:
    api_key_pepper: test-merchant-terminal-api-key-pepper-32b
    rotation:
      default_grace_period: 24h
      max_grace_period: 168h
    issuance_delivery_ttl: 24h
    issuance_encryption_keys: m1:dGVzdC1tZXJjaGFudC1hdXRoLWlzc3VhbmNlLWtleSE=
infrastructure:
  clock:
    location: UTC
    business_location: UTC
  logger:
    provider: noop
  http_client:
    provider: noop
  cache:
    provider: noop
  messaging:
    provider: noop
  task:
    provider: inprocess
    inprocess:
      default_timezone: UTC
    scheduled:
      saga_executor:
        enabled: true
        handler: saga_executor
        trigger:
          type: interval
          interval: 10s
  circuit_breaker:
    provider: noop
external:
  credit_assessment:
    provider: noop
  webhook_dispatcher:
    provider: http
    noop:
    http:
      timeout: 10s
      dial_timeout: 5s
      max_response_bytes: 65536
  financial_core:
    provider: noop
    payment_return_url: https://bnpl.example/payments/return
  customer:
    provider: noop
  notification:
    provider: noop
`

func writeConfigFile(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}

func TestLoad_ValidMinimalConfig(t *testing.T) {
	t.Parallel()
	skipIfProductionBinary(t)

	cfg, err := Load(writeConfigFile(t, validVersionsYAML+validMinimalValidationYAML))
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, Version, cfg.Versions.Config)
	assert.Equal(t, DatabaseVersion, cfg.Versions.Database)
	assert.Equal(t, "http", cfg.External.WebhookDispatcher.Provider)
	assert.Equal(t, 10*time.Second, cfg.External.WebhookDispatcher.HTTP.Timeout)
	assert.Equal(t, 5*time.Second, cfg.External.WebhookDispatcher.HTTP.DialTimeout)
	assert.Equal(t, int64(65536), cfg.External.WebhookDispatcher.HTTP.MaxResponseBytes)
}

func TestLoad_FullConfigSections(t *testing.T) {
	t.Parallel()
	skipIfProductionBinary(t)

	content := validVersionsYAML + strings.Replace(
		strings.Replace(
			strings.Replace(
				strings.Replace(validMinimalValidationYAML,
					"    worker_id: worker-1", "    worker_id: worker-a", 1),
				"    batch_size: 10", "    batch_size: 50", 1),
			"  payment_attempt_reconciliation:",
			`  health:
    enabled: true
  payment_attempt_reconciliation:`, 1),
		"  circuit_breaker:",
		`  shutdown:
    timeout: 45s
  health:
    checks:
      - database
  circuit_breaker:`, 1) + `
gin:
  host: 0.0.0.0
  port: 8080
  logger: true
  recovery: true
  swagger:
    enabled: true
  http_debug_logger:
    enabled: false
    max_request_body_bytes: 65536
    max_response_body_bytes: 65536
  response_logging:
    enabled: false
    max_body_bytes: 16384
`

	cfg, err := Load(writeConfigFile(t, content))
	require.NoError(t, err)
	assert.Equal(t, "localhost", cfg.Database.Host)
	assert.Equal(t, 5432, cfg.Database.Port)
	assert.Equal(t, "bnpl", cfg.Database.Name)
	assert.Equal(t, "0.0.0.0", cfg.Gin.Host)
	assert.Equal(t, 8080, cfg.Gin.Port)
	assert.True(t, cfg.Gin.Logger)
	assert.True(t, cfg.Gin.Swagger.Enabled)
	assert.False(t, cfg.Gin.HTTPDebugLogger.Enabled)
	assert.Equal(t, int64(65536), cfg.Gin.HTTPDebugLogger.MaxRequestBodyBytes)
	assert.Equal(t, int64(65536), cfg.Gin.HTTPDebugLogger.MaxResponseBodyBytes)
	assert.False(t, cfg.Gin.ResponseLogging.Enabled)
	assert.Equal(t, int64(16384), cfg.Gin.ResponseLogging.MaxBodyBytes)
	assert.True(t, cfg.Usecases.Health.Enabled)
	assert.Equal(t, "worker-a", cfg.Usecases.OutboxPublisher.WorkerID)
	assert.Equal(t, 50, cfg.Usecases.OutboxPublisher.BatchSize)
}

func TestLoad_ExpandsEnvironmentVariables(t *testing.T) {
	skipIfProductionBinary(t)

	t.Setenv("TEST_DB_HOST", "postgres.local")
	t.Setenv("TEST_GIN_PORT", "9090")

	content := validVersionsYAML + strings.Replace(validMinimalValidationYAML, "  host: localhost", "  host: ${TEST_DB_HOST}", 1) + `
gin:
  host: 0.0.0.0
  port: ${TEST_GIN_PORT}
  logger: true
  recovery: true
`

	cfg, err := Load(writeConfigFile(t, content))
	require.NoError(t, err)
	assert.Equal(t, "postgres.local", cfg.Database.Host)
	assert.Equal(t, 9090, cfg.Gin.Port)
}

func TestLoad_MissingFile(t *testing.T) {
	t.Parallel()

	_, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading config file")
}

func TestLoad_InvalidYAML(t *testing.T) {
	t.Parallel()

	_, err := Load(writeConfigFile(t, ":\n  bad: ["))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshalling yaml")
}

func TestLoad_EmptyFile(t *testing.T) {
	t.Parallel()

	_, err := Load(writeConfigFile(t, ""))
	require.Error(t, err)
}

func TestLoad_ScalarRootDocument(t *testing.T) {
	t.Parallel()

	_, err := Load(writeConfigFile(t, "not-a-mapping"))
	require.Error(t, err)
}

func TestLoad_UnsupportedConfigVersion(t *testing.T) {
	t.Parallel()

	content := `
application: runtime
versions:
  database: 0.2.0
  config: 0.9.0
`

	_, err := Load(writeConfigFile(t, content))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "validating versions")
	assert.Contains(t, err.Error(), "unsupported minor version")
}

func TestLoad_MissingVersions(t *testing.T) {
	t.Parallel()

	_, err := Load(writeConfigFile(t, "application: runtime\ndatabase:\n  host: localhost\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "validating versions")
}

func TestConfig_ValidateVersions(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Versions: Versions{
			Config:   Version,
			Database: DatabaseVersion,
		},
	}

	require.NoError(t, cfg.ValidateVersions())
}

func TestConfig_String_RedactsSecrets(t *testing.T) {
	t.Parallel()
	skipIfProductionBinary(t)

	content := validVersionsYAML + strings.Replace(
		strings.Replace(validMinimalValidationYAML,
			"admin: localhost:5432:bnpl:admin:secret", "admin: localhost:5432:bnpl:admin:secret-value", 1),
		"normal: localhost:5432:bnpl:bnpl:secret", "normal: localhost:5432:bnpl:bnpl:secret-value", 1)
	loaded, err := Load(writeConfigFile(t, content))
	require.NoError(t, err)

	out := loaded.String()
	require.NotEmpty(t, out)
	assert.NotContains(t, out, "secret-value")
	assert.Contains(t, out, "REDACTED")
}
