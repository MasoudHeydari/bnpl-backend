package migrationsuc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/clock"
	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

const (
	nonTransactionalMigrationDirective = "-- bnpl:transaction=off"
	defaultMigrationLockTimeout        = 30 * time.Second
)

// UseCase orchestrates filesystem-backed migrations against the target store.
type UseCase struct {
	pool        repo.Pool
	migrations  repo.Migrations
	clock       clock.Clock
	logger      log.Logger
	migDirPath  string
	lockTimeout time.Duration
}

// New constructs a migration use case.
func New(
	pool repo.Pool,
	migrations repo.Migrations,
	clk clock.Clock,
	migDirPath string,
	logger log.Logger,
) (*UseCase, error) {
	if pool == nil {
		return nil, fmt.Errorf("pool is required")
	}

	if migrations == nil {
		return nil, fmt.Errorf("migrations repository is required")
	}

	if clk == nil {
		return nil, fmt.Errorf("clock is required")
	}

	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	return &UseCase{
		pool:        pool,
		migrations:  migrations,
		clock:       clk,
		logger:      logger,
		migDirPath:  migDirPath,
		lockTimeout: defaultMigrationLockTimeout,
	}, nil
}

// WithLockTimeout configures how long the runner may wait for the database-wide
// migration advisory lock. Non-positive values preserve the safe default.
func (uc *UseCase) WithLockTimeout(timeout time.Duration) *UseCase {
	if timeout > 0 {
		uc.lockTimeout = timeout
	}
	return uc
}

type migrationFile struct {
	name             string
	statements       string
	checksum         string
	nonTransactional bool
}

// MigrateAll applies regular .sql files in lexical order while holding a
// database-wide advisory lock. Already-applied files are skipped only when the
// persisted checksum exactly matches their current contents.
func (uc *UseCase) MigrateAll(ctx context.Context) error {
	const op = "migrationsuc.UseCase.MigrateAll"

	files, err := uc.readMigrationFiles()
	if err != nil {
		return err
	}

	uc.logger.Info(
		"migration run started",
		"migration_dir", uc.migDirPath,
		"file_count", len(files),
	)

	var (
		appliedCount int
		skippedCount int
	)

	err = uc.withMigrationLock(ctx, func(
		ctx context.Context,
		conn repo.Conn,
		q repo.MigrationsConnQueryer,
	) error {
		applied, err := q.ListApplied(ctx)
		if err != nil {
			return fmt.Errorf("%s: list applied migrations: %w", op, err)
		}

		appliedByName := make(map[string]model.Migration, len(applied))
		for _, migration := range applied {
			appliedByName[migration.Name] = migration
		}

		for _, file := range files {
			if existing, ok := appliedByName[file.name]; ok {
				if existing.Dirty {
					uc.logger.Error(
						"migration is dirty; refusing to continue",
						"migration", existing.Name,
						"checksum", existing.Checksum,
					)

					return dirtyMigrationError(op, existing)
				}

				if isLegacyMigrationChecksum(existing.Checksum) {
					uc.logger.Info(
						"migration already applied (legacy checksum)",
						"migration", file.name,
						"checksum", existing.Checksum,
					)
					skippedCount++

					continue
				}

				if existing.Checksum != file.checksum {
					uc.logger.Error(
						"migration checksum mismatch",
						"migration", file.name,
						"database_checksum", existing.Checksum,
						"filesystem_checksum", file.checksum,
					)

					return fmt.Errorf(
						"%s: migration %q checksum mismatch: database=%q filesystem=%q", op, file.name,
						existing.Checksum,
						file.checksum,
					)
				}

				uc.logger.Info(
					"migration already applied",
					"migration", file.name,
					"checksum", file.checksum,
				)
				skippedCount++

				continue
			}

			if err := uc.applyMigration(ctx, conn, q, file); err != nil {
				return err
			}

			appliedCount++
		}

		return nil
	})
	if err != nil {
		uc.logger.Error(
			"migration run failed",
			"migration_dir", uc.migDirPath,
			"applied_count", appliedCount,
			"skipped_count", skippedCount,
			"error", err.Error(),
		)

		return err
	}

	uc.logger.Info(
		"migration run completed",
		"migration_dir", uc.migDirPath,
		"file_count", len(files),
		"applied_count", appliedCount,
		"skipped_count", skippedCount,
	)

	return nil
}

// MigrateOne applies a single regular .sql migration under the same locking and
// checksum rules used by MigrateAll.
func (uc *UseCase) MigrateOne(ctx context.Context, migName string) error {
	const op = "migrationsuc.UseCase.MigrateOne"

	file, err := uc.readMigrationFile(migName)
	if err != nil {
		return err
	}

	uc.logger.Info(
		"migration one started",
		"migration", file.name,
		"checksum", file.checksum,
	)

	err = uc.withMigrationLock(ctx, func(
		ctx context.Context,
		conn repo.Conn,
		q repo.MigrationsConnQueryer,
	) error {
		applied, err := q.ListApplied(ctx)
		if err != nil {
			return fmt.Errorf("%s: list applied migrations: %w", op, err)
		}

		for _, existing := range applied {
			if existing.Name != file.name {
				continue
			}

			if existing.Dirty {
				uc.logger.Error(
					"migration is dirty; refusing to continue",
					"migration", existing.Name,
					"checksum", existing.Checksum,
				)

				return dirtyMigrationError(op, existing)
			}

			if isLegacyMigrationChecksum(existing.Checksum) {
				uc.logger.Info(
					"migration already applied (legacy checksum)",
					"migration", file.name,
					"checksum", existing.Checksum,
				)

				return nil
			}

			if existing.Checksum != file.checksum {
				uc.logger.Error(
					"migration checksum mismatch",
					"migration", file.name,
					"database_checksum", existing.Checksum,
					"filesystem_checksum", file.checksum,
				)

				return fmt.Errorf(
					"%s: migration %q checksum mismatch: database=%q filesystem=%q", op, file.name,
					existing.Checksum,
					file.checksum,
				)
			}

			uc.logger.Info(
				"migration already applied",
				"migration", file.name,
				"checksum", file.checksum,
			)

			return nil
		}

		return uc.applyMigration(ctx, conn, q, file)
	})
	if err != nil {
		uc.logger.Error(
			"migration one failed",
			"migration", file.name,
			"checksum", file.checksum,
			"error", err.Error(),
		)

		return err
	}

	return nil
}

func (uc *UseCase) withMigrationLock(
	ctx context.Context,
	handler func(context.Context, repo.Conn, repo.MigrationsConnQueryer) error,
) error {
	return uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) (retErr error) {
		q := uc.migrations.Conn(conn)

		lockCtx, cancelLock := context.WithTimeout(ctx, uc.lockTimeout)
		err := q.AcquireLock(lockCtx)
		cancelLock()
		if err != nil {
			return fmt.Errorf("acquire migration lock within %s: %w", uc.lockTimeout, err)
		}

		defer func() {
			// Releasing with a fresh bounded context avoids retaining a session lock
			// when the caller deadline expires during a migration failure.
			releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			retErr = errors.Join(retErr, q.ReleaseLock(releaseCtx))
		}()

		return handler(ctx, conn, q)
	})
}

func (uc *UseCase) applyMigration(
	ctx context.Context,
	conn repo.Conn,
	connQ repo.MigrationsConnQueryer,
	file migrationFile,
) error {
	const op = "migrationsuc.UseCase.applyMigration"

	mode := "transactional"
	if file.nonTransactional {
		mode = "non_transactional"
	}

	logger := uc.logger.With(
		"migration", file.name,
		"checksum", file.checksum,
		"mode", mode,
	)

	logger.Info("applying migration")

	if file.nonTransactional {
		startedAt := uc.clock.Now()
		if _, err := connQ.MarkDirty(ctx, file.name, file.checksum, startedAt); err != nil {
			logger.Error("failed to mark migration dirty", "error", err.Error())

			return fmt.Errorf("%s: mark non-transactional migration %q dirty: %w", op, file.name, err)
		}

		if err := connQ.Migrate(ctx, file.name, file.statements); err != nil {
			logger.Error(
				"migration apply failed; remains dirty and requires operator verification",
				"error", err.Error(),
			)

			return fmt.Errorf(
				"%s: apply non-transactional migration %q; migration remains dirty and requires operator verification: %w",
				op, file.name, err,
			)
		}

		if _, err := connQ.MarkApplied(ctx, file.name, file.checksum, uc.clock.Now()); err != nil {
			logger.Error(
				"migration finalize failed; remains dirty and requires operator verification",
				"error", err.Error(),
			)

			return fmt.Errorf(
				"%s: finalize non-transactional migration %q; migration remains dirty and requires operator verification: %w",
				op, file.name, err,
			)
		}

		logger.Info("migration applied")

		return nil
	}

	err := conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
		q := uc.migrations.Tx(tx)
		if err := q.Migrate(ctx, file.name, file.statements); err != nil {
			return fmt.Errorf("%s: apply migration %q: %w", op, file.name, err)
		}

		if _, err := q.Register(ctx, file.name, file.checksum, uc.clock.Now()); err != nil {
			return fmt.Errorf("%s: register migration %q: %w", op, file.name, err)
		}

		return nil
	})
	if err != nil {
		logger.Error("migration apply failed", "error", err.Error())

		return err
	}

	logger.Info("migration applied")

	return nil
}

func (uc *UseCase) readMigrationFiles() ([]migrationFile, error) {
	const op = "migrationsuc.UseCase.readMigrationFiles"

	entries, err := os.ReadDir(uc.migDirPath)
	if err != nil {
		return nil, fmt.Errorf("%s: read migration directory: %w", op, err)
	}

	files := make([]migrationFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || strings.ToLower(filepath.Ext(entry.Name())) != ".sql" {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("%s: stat migration %q: %w", op, entry.Name(), err)
		}

		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			continue
		}

		file, err := uc.readMigrationFile(entry.Name())
		if err != nil {
			return nil, err
		}

		files = append(files, file)
	}

	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })
	return files, nil
}

func (uc *UseCase) readMigrationFile(name string) (migrationFile, error) {
	const op = "migrationsuc.UseCase.readMigrationFile"

	if filepath.Base(name) != name || strings.ToLower(filepath.Ext(name)) != ".sql" {
		return migrationFile{}, fmt.Errorf("%s: invalid migration filename %q", op, name)
	}

	path := filepath.Join(uc.migDirPath, name)
	info, err := os.Lstat(path)
	if err != nil {
		return migrationFile{}, fmt.Errorf("%s: stat migration %q: %w", op, name, err)
	}

	if !info.Mode().IsRegular() {
		return migrationFile{}, fmt.Errorf("%s: migration %q is not a regular file", op, name)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return migrationFile{}, fmt.Errorf("%s: read migration %q: %w", op, name, err)
	}

	if len(strings.TrimSpace(string(raw))) == 0 {
		return migrationFile{}, fmt.Errorf("%s: migration %q is empty", op, name)
	}

	digest := sha256.Sum256(raw)
	return migrationFile{
		name:             name,
		statements:       string(raw),
		checksum:         "sha256:" + hex.EncodeToString(digest[:]),
		nonTransactional: hasNonTransactionalDirective(string(raw)),
	}, nil
}

func isLegacyMigrationChecksum(checksum string) bool {
	return strings.HasPrefix(checksum, "legacy:")
}

func hasNonTransactionalDirective(statements string) bool {
	for _, line := range strings.Split(statements, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		return trimmed == nonTransactionalMigrationDirective
	}

	return false
}

func dirtyMigrationError(op string, migration model.Migration) error {
	return fmt.Errorf(
		"%s: migration %q is dirty (checksum=%q); verify the database state and resolve it explicitly before retrying",
		op,
		migration.Name,
		migration.Checksum,
	)
}
