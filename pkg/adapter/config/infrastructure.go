package config

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log"
	lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"
	adapterslog "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/slog"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/trace"
	coreclock "github.com/parstis/bnpl-backend/pkg/core/clock"
	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
)

type Infrastructure struct {
	Trace  Trace
	Clock  Clock
	Logger Logger
}

type Trace struct{}

func (t Trace) newTraceInfrastructure() (*trace.Provider, error) {
	return trace.NewULIDProvider(), nil
}

type Clock struct {
	Location         string `yaml:"location"`
	BusinessLocation string `yaml:"business_location"`
}

func (c Clock) newSystemClockInfrastructure() (coreclock.Clock, error) {
	location, err := time.LoadLocation(c.Location)
	if err != nil {
		return nil, fmt.Errorf(
			"invalid infrastructure.clock.location %q: %w",
			c.Location,
			err,
		)
	}

	businessLocation, err := time.LoadLocation(c.BusinessLocation)
	if err != nil {
		return nil, fmt.Errorf(
			"invalid infrastructure.clock.business_location %q: %w",
			c.BusinessLocation,
			err,
		)
	}

	return clock.NewSystem(location, businessLocation), nil
}

// Logger configures the logging infrastructure.
type Logger struct {
	Provider log.Provider `yaml:"provider"`
	Noop     Noop         `yaml:"noop"`
	Slog     Slog         `yaml:"slog"`
}

// Slog configures the slog logger.
type Slog struct {
	// Level controls log verbosity.
	Level corelog.Level `yaml:"level"`

	// Format controls log output encoding.
	Format corelog.Format `yaml:"format"`
}

// newLoggerInfrastructure creates the configured logger.
func (l Logger) newLoggerInfrastructure() (corelog.Logger, error) {
	switch l.Provider {
	case log.SlogProvider:
		return l.newSlogLoggerInfrastructure()

	case log.NoopProvider:
		return l.newNoopLoggerInfrastructure()

	default:
		return nil, fmt.Errorf("unsupported log provider: %q", l.Provider)
	}
}

// newNoopLoggerInfrastructure creates a logger that discards all records.
func (l Logger) newNoopLoggerInfrastructure() (corelog.Logger, error) {
	return lognoop.New(), nil
}

// newSlogLoggerInfrastructure creates a slog-backed logger.
func (l Logger) newSlogLoggerInfrastructure() (corelog.Logger, error) {
	level, err := l.slogLevel()
	if err != nil {
		return nil, err
	}

	format, err := l.logFormat()
	if err != nil {
		return nil, err
	}

	return adapterslog.New(os.Stdout, level, format), nil
}

// slogLevel converts config log level to slog level.
func (l Logger) slogLevel() (slog.Level, error) {
	switch l.Slog.Level {
	case "":
		return slog.LevelInfo, nil
	case corelog.DebugLevel:
		return slog.LevelDebug, nil
	case corelog.InfoLevel:
		return slog.LevelInfo, nil
	case corelog.WarnLevel:
		return slog.LevelWarn, nil
	case corelog.ErrorLevel:
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("invalid log level: %q", l.Slog.Level)
	}
}

// logFormat validates log output format.
func (l Logger) logFormat() (corelog.Format, error) {
	switch l.Slog.Format {
	case "":
		return corelog.JSONFormat, nil
	case corelog.JSONFormat, corelog.TextFormat:
		return l.Slog.Format, nil
	default:
		return corelog.JSONFormat, fmt.Errorf("invalid log format: %q", l.Slog.Format)
	}
}
