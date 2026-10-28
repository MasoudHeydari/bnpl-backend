package bootstrap

import (
	"context"
	"fmt"
	"os"
	"time"

	coreerrorreporting "github.com/parstis/bnpl-backend/pkg/core/errorreporting"
	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
)

// cfgPath is the Runtime configuration path used by Boot and business CLI
// workflows. Set it through FixConfigPath before invoking those entrypoints.
var cfgPath string

const commandPanicFlushTimeout = 2 * time.Second

func recoverCommandPanic(
	ctx context.Context,
	command string,
	logger corelog.Logger,
	reporter coreerrorreporting.Reporter,
	err *error,
) {
	const op = "bootstrap.recoverCommandPanic"

	recovered := recover()
	if recovered == nil {
		return
	}

	if reporter != nil {
		reporter.CapturePanic(
			ctx,
			recovered,
			coreerrorreporting.Metadata{
				Component: "cli",
				Tags: map[string]string{
					"cli.command": command,
				},
			},
		)

		flushCtx, cancel := context.WithTimeout(context.Background(), commandPanicFlushTimeout)
		reporter.Flush(flushCtx)
		cancel()
	}

	if logger != nil {
		logger.With(
			"command", command,
			"panic", fmt.Sprint(recovered),
		).Error("cli command panic recovered")
	}

	if err != nil {
		*err = fmt.Errorf("%s: %s panic recovered: %v", op, command, recovered)
	}
}

// FixConfigPath resolves the configuration file path from the CLI flag,
// CONFIG_FILE environment variable, or the repository default.
func FixConfigPath(cliPath string) {
	if cliPath != "" {
		cfgPath = cliPath
		return
	}

	if path, found := os.LookupEnv("CONFIG_FILE"); found {
		cfgPath = path
		return
	}

	cfgPath = "configs/runtime.yaml"
}
