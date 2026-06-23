package config

import (
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/clock"
	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/messaging"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/migrationsuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/outboxpublisheruc"
)

// Usecases groups configuration sections used to build application-layer
// use cases.
//
// This struct is part of the adapter/config layer: it translates YAML
// configuration into constructor inputs, but it must not contain business
// rules itself.
type Usecases struct {
	Migrations      Migrations      `yaml:"migration"`
	OutboxPublisher OutboxPublisher `yaml:"outbox_publisher"`
}

// Migrations contains filesystem-backed migration settings.
//
// filesDir points to the directory that contains ordered SQL migration files
// used by `bnpl db migrate`. The path is read from:
//
//	usecases:
//	  migration:
//	    files-dir: ...
//
// The field is intentionally unexported because callers should construct the
// migration use case through the config package instead of reading raw config.
type Migrations struct {
	filesDir string `yaml:"files-dir"`
}

// newUseCase constructs a migration use case from the configured migration
// directory, database pool, and migration repository.
//
// The caller retains ownership of p and r. The returned use case uses filesDir
// only as a filesystem path and does not validate the directory until migration
// execution.
func (m Migrations) newUseCase(
	p repo.Pool, r repo.Migrations,
) *migrationsuc.UseCase {
	return migrationsuc.New(p, r, m.filesDir)
}

// OutboxPublisher contains outbox publishing use case settings.
type OutboxPublisher struct {
	// WorkerID uniquely identifies the worker instance claiming outbox rows.
	//
	// Multiple workers may safely process the same outbox table concurrently.
	WorkerID string `yaml:"worker_id"`

	// BatchSize is the maximum number of messages claimed in a single run.
	BatchSize int `yaml:"batch_size"`

	// RetryBaseDelay is the initial retry delay used for exponential backoff.
	RetryBaseDelay time.Duration `yaml:"retry_base_delay"`

	// RetryMaxDelay is the upper bound for exponential backoff delay.
	RetryMaxDelay time.Duration `yaml:"retry_max_delay"`
}

// newUseCase creates the outbox publisher use case using the configured
// retry policy, batch size, and worker identity.
func (obp OutboxPublisher) newUseCase(
	pool repo.Pool,
	outbox repo.Outbox,
	publisher messaging.Publisher,
	clock clock.Clock,
	logger log.Logger,
) *outboxpublisheruc.UseCase {
	return outboxpublisheruc.New(
		pool,
		outbox,
		publisher,
		clock,
		logger,
		obp.WorkerID,
		obp.BatchSize,
		obp.RetryBaseDelay,
		obp.RetryMaxDelay,
	)
}
