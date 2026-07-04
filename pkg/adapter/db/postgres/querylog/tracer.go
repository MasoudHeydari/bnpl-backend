package querylog

import (
	"context"

	"github.com/jackc/pgx/v5"

	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
)

var _ pgx.QueryTracer = (*Tracer)(nil)

// Tracer implements pgx.QueryTracer and emits one log record per query start.
//
// Fields:
//   - logger: destination for structured query records. Required; New panics if nil.
//   - includeArguments: when true, positional $1..$N placeholders are replaced
//     with SQL literals in the logged text only (see Render).
//
// Tracer is safe for concurrent use: it holds no shared mutable state and each
// TraceQueryStart call only reads its config and writes through the logger.
type Tracer struct {
	logger           corelog.Logger
	includeArguments bool
}

// New constructs a PostgreSQL query tracer.
//
// logger must be non-nil; callers only construct a Tracer when query logging is
// enabled and a logger is available. includeArguments controls whether Render
// runs before logging.
func New(logger corelog.Logger, includeArguments bool) *Tracer {
	if logger == nil {
		panic("querylog: logger is nil")
	}

	return &Tracer{
		logger:           logger,
		includeArguments: includeArguments,
	}
}

// TraceQueryStart logs the SQL statement before pgx executes it.
//
// It returns the incoming context unchanged. pgx allows tracers to stash
// per-query values in context for TraceQueryEnd; this implementation logs
// everything at start, so no context enrichment is required.
func (t *Tracer) TraceQueryStart(
	ctx context.Context,
	_ *pgx.Conn,
	data pgx.TraceQueryStartData,
) context.Context {
	sqlText := data.SQL
	if t.includeArguments && len(data.Args) > 0 {
		rendered, err := Render(data.SQL, data.Args)
		if err != nil {
			// Fall back to the parameterized statement so a rendering bug never
			// hides the fact that a query ran.
			t.logger.Warn(
				"database query argument rendering failed",
				"sql", data.SQL,
				"argument_count", len(data.Args),
				"error", err,
			)
		} else {
			sqlText = rendered
		}
	}

	// INFO (not DEBUG): query logging is an explicit operator opt-in. Using INFO
	// keeps enabled traces visible even when the process-wide log level is INFO.
	t.logger.Info("database query", "sql", sqlText)

	return ctx
}

// TraceQueryEnd completes the pgx.QueryTracer contract.
//
// Duration, command tags, and errors are intentionally not logged here. The
// goal of this package is to show which SQL was issued, not to replace metrics
// or error handling already covered elsewhere in the stack.
func (t *Tracer) TraceQueryEnd(
	context.Context,
	*pgx.Conn,
	pgx.TraceQueryEndData,
) {
}
